// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hostdcloud/terraform-provider-hostd/internal/client"
)

var (
	_ resource.Resource                   = &securityGroupResource{}
	_ resource.ResourceWithConfigure      = &securityGroupResource{}
	_ resource.ResourceWithImportState    = &securityGroupResource{}
	_ resource.ResourceWithModifyPlan     = &securityGroupResource{}
	_ resource.ResourceWithValidateConfig = &securityGroupResource{}
)

func NewSecurityGroupResource() resource.Resource {
	return &securityGroupResource{}
}

type securityGroupResource struct {
	client *client.Client
}

type securityGroupResourceModel struct {
	ID           types.String `tfsdk:"id"`
	ProjectID    types.String `tfsdk:"project_id"`
	Name         types.String `tfsdk:"name"`
	Comment      types.String `tfsdk:"comment"`
	PVEGroupName types.String `tfsdk:"pve_group_name"`
	Rules        types.List   `tfsdk:"rules"`
}

func (r *securityGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_group"
}

func (r *securityGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *securityGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A project firewall security group and its ordered rules. Attach it to a VM with a `type = \"group\"` rule in `hostd_vm_firewall`. " +
			"`rules` is the complete list: rules added outside Terraform are removed on the next apply. " +
			"Deleting the group removes its rules and detaches it from every VM in the project. " +
			"Renaming it changes `pve_group_name`; VM attachments follow. Import with `<project_id>,<id>`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Security group UUID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the project (`hostd_project`) the group belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name: a lowercase letter, then up to 14 letters, digits, hyphens or underscores.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(ipSetNamePattern, "must start with a lowercase letter and contain only lowercase letters, digits, hyphens and underscores, at most 15 characters"),
				},
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "Free-text comment. Empty by default.",
			},
			"pve_group_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier a `hostd_vm_firewall` rule uses as `action` when its type is `group`. Changes when the group is renamed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"rules": firewallRulesAttribute("Rules in evaluation order. Empty by default.", false),
		},
	}
}

func (r *securityGroupResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var rules types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("rules"), &rules)...)
	resp.Diagnostics.Append(validateFirewallRules(ctx, rules, path.Root("rules"))...)
}

// ModifyPlan marks pve_group_name unknown on a rename: the API assigns a new one.
func (r *securityGroupResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var planName, stateName types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("name"), &planName)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("name"), &stateName)...)
	if !planName.Equal(stateName) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("pve_group_name"), types.StringUnknown())...)
	}
}

func (r *securityGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data securityGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rules, diags := firewallRuleInputs(ctx, data.Rules)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID := data.ProjectID.ValueString()
	group, err := r.client.CreateSecurityGroup(ctx, projectID, data.Name.ValueString(), data.Comment.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating security group", fmt.Sprintf("creating security group (%s): %s", data.Name.ValueString(), err))
		return
	}
	data.ID = types.StringValue(group.UUID)
	data.Name = types.StringValue(group.DisplayName)
	data.Comment = types.StringValue(group.Comment)
	data.PVEGroupName = types.StringValue(group.PVEGroupName)

	if len(rules) > 0 {
		// Track the group before the rules, so a failure leaves it tainted instead of orphaned.
		planned := data.Rules
		data.Rules = emptyFirewallRules()
		resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		if err := r.client.ReplaceSecurityGroupRules(ctx, projectID, group.UUID, rules); err != nil {
			resp.Diagnostics.AddError("Error setting security group rules", fmt.Sprintf("setting rules of security group %s: %s", group.UUID, err))
			return
		}
		data.Rules = planned
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *securityGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data securityGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID, id := data.ProjectID.ValueString(), data.ID.ValueString()
	group, err := r.client.GetSecurityGroup(ctx, projectID, id)
	var rules []client.FirewallRule
	if err == nil {
		rules, err = r.client.ListSecurityGroupRules(ctx, projectID, id)
	}
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading security group", fmt.Sprintf("reading security group (%s): %s", id, err))
		return
	}
	data.Name = keepConfigured(data.Name, group.DisplayName)
	data.Comment = keepConfigured(data.Comment, group.Comment)
	data.PVEGroupName = types.StringValue(group.PVEGroupName)
	var diags diag.Diagnostics
	data.Rules, diags = firewallRulesFromAPI(ctx, data.Rules, rules)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *securityGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state securityGroupResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID, id := plan.ProjectID.ValueString(), plan.ID.ValueString()
	plan.PVEGroupName = state.PVEGroupName
	if !plan.Name.Equal(state.Name) || !plan.Comment.Equal(state.Comment) {
		group, err := r.client.UpdateSecurityGroup(ctx, projectID, id, plan.Name.ValueString(), plan.Comment.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Error updating security group", fmt.Sprintf("updating security group (%s): %s", id, err))
			return
		}
		plan.PVEGroupName = types.StringValue(group.PVEGroupName)
	}
	if !plan.Rules.Equal(state.Rules) {
		rules, diags := firewallRuleInputs(ctx, plan.Rules)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		if err := r.client.ReplaceSecurityGroupRules(ctx, projectID, id, rules); err != nil {
			resp.Diagnostics.AddError("Error setting security group rules", fmt.Sprintf("setting rules of security group %s: %s", id, err))
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *securityGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data securityGroupResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteSecurityGroup(ctx, data.ProjectID.ValueString(), data.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting security group", fmt.Sprintf("deleting security group (%s): %s", data.ID.ValueString(), err))
	}
}

func (r *securityGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectID, id, ok := strings.Cut(req.ID, ",")
	if !ok || projectID == "" || id == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected <project_id>,<id>, got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
