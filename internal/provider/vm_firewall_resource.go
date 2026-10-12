// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hostdcloud/terraform-provider-hostd/internal/client"
)

var (
	_ resource.Resource                   = &vmFirewallResource{}
	_ resource.ResourceWithConfigure      = &vmFirewallResource{}
	_ resource.ResourceWithImportState    = &vmFirewallResource{}
	_ resource.ResourceWithValidateConfig = &vmFirewallResource{}
)

// Proxmox defaults for a VM that never had firewall options set; a new VM has these.
const (
	defaultPolicyIn  = "DROP"
	defaultPolicyOut = "ACCEPT"
)

func NewVMFirewallResource() resource.Resource {
	return &vmFirewallResource{}
}

type vmFirewallResource struct {
	client *client.Client
}

type vmFirewallResourceModel struct {
	ProjectID types.String `tfsdk:"project_id"`
	VMID      types.Int64  `tfsdk:"vmid"`
	Enable    types.Bool   `tfsdk:"enable"`
	PolicyIn  types.String `tfsdk:"policy_in"`
	PolicyOut types.String `tfsdk:"policy_out"`
	Rules     types.List   `tfsdk:"rules"`
}

func (r *vmFirewallResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_firewall"
}

func (r *vmFirewallResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *vmFirewallResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The firewall of one VM: whether it is enabled, the default policies, and the ordered rules and attached security groups. " +
			"The firewall filters the public interface. Use one `hostd_vm_firewall` per VM; `rules` is the complete list, so rules added outside Terraform are removed on the next apply. " +
			"Rules are written before the options, so enabling the firewall with `policy_in = \"DROP\"` does not cut off traffic the rules allow. " +
			"Destroy removes every rule and turns the firewall off, as on a new VM. Import with `<project_id>,<vmid>`.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the project the VM belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vmid": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "VM identifier (`hostd_vm.vmid`).",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"enable": schema.BoolAttribute{
				Required:            true,
				MarkdownDescription: "Whether the VM firewall is enabled.",
			},
			"policy_in": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Default policy for inbound traffic: `ACCEPT` or `DROP`.",
				Validators: []validator.String{
					stringvalidator.OneOf("ACCEPT", "DROP"),
				},
			},
			"policy_out": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Default policy for outbound traffic: `ACCEPT` or `DROP`.",
				Validators: []validator.String{
					stringvalidator.OneOf("ACCEPT", "DROP"),
				},
			},
			"rules": firewallRulesAttribute("Rules and attached security groups in evaluation order. Empty by default.", true),
		},
	}
}

func (r *vmFirewallResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var rules types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("rules"), &rules)...)
	resp.Diagnostics.Append(validateFirewallRules(ctx, rules, path.Root("rules"))...)
}

// apply writes the rules first, then the options.
func (r *vmFirewallResource) apply(ctx context.Context, plan vmFirewallResourceModel, state *vmFirewallResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	projectID, vmid := plan.ProjectID.ValueString(), plan.VMID.ValueInt64()
	if state == nil || !plan.Rules.Equal(state.Rules) {
		rules, more := firewallRuleInputs(ctx, plan.Rules)
		diags.Append(more...)
		if diags.HasError() {
			return diags
		}
		if err := r.client.ReplaceVMFirewallRules(ctx, projectID, vmid, rules); err != nil {
			diags.AddError("Error setting VM firewall rules", fmt.Sprintf("setting firewall rules of VM %d: %s", vmid, err))
			return diags
		}
	}
	if state == nil || !plan.Enable.Equal(state.Enable) || !plan.PolicyIn.Equal(state.PolicyIn) || !plan.PolicyOut.Equal(state.PolicyOut) {
		err := r.client.SetVMFirewallOptions(ctx, projectID, vmid, plan.Enable.ValueBool(), plan.PolicyIn.ValueString(), plan.PolicyOut.ValueString())
		if err != nil {
			diags.AddError("Error setting VM firewall options", fmt.Sprintf("setting firewall options of VM %d: %s", vmid, err))
		}
	}
	return diags
}

func (r *vmFirewallResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data vmFirewallResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, data, nil)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *vmFirewallResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data vmFirewallResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID, vmid := data.ProjectID.ValueString(), data.VMID.ValueInt64()
	options, err := r.client.GetVMFirewallOptions(ctx, projectID, vmid)
	var rules []client.FirewallRule
	if err == nil {
		rules, err = r.client.ListVMFirewallRules(ctx, projectID, vmid)
	}
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading VM firewall", fmt.Sprintf("reading firewall of VM %d: %s", vmid, err))
		return
	}
	data.Enable = types.BoolValue(options.Enable != nil && *options.Enable)
	data.PolicyIn = types.StringValue(valueOr(options.PolicyIn, defaultPolicyIn))
	data.PolicyOut = types.StringValue(valueOr(options.PolicyOut, defaultPolicyOut))
	var diags diag.Diagnostics
	data.Rules, diags = firewallRulesFromAPI(ctx, data.Rules, rules)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *vmFirewallResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vmFirewallResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, plan, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vmFirewallResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data vmFirewallResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID, vmid := data.ProjectID.ValueString(), data.VMID.ValueInt64()
	err := r.client.ReplaceVMFirewallRules(ctx, projectID, vmid, []client.FirewallRuleInput{})
	if err == nil {
		err = r.client.SetVMFirewallOptions(ctx, projectID, vmid, false, defaultPolicyIn, defaultPolicyOut)
	}
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error resetting VM firewall", fmt.Sprintf("resetting firewall of VM %d: %s", vmid, err))
	}
}

func (r *vmFirewallResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectID, rawVMID, ok := strings.Cut(req.ID, ",")
	vmid, err := strconv.ParseInt(rawVMID, 10, 64)
	if !ok || projectID == "" || err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected <project_id>,<vmid>, got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vmid"), vmid)...)
}

func valueOr(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}
