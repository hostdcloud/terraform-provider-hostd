// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hostdcloud/terraform-provider-hostd/internal/client"
)

var (
	_ resource.Resource                = &securityGroupRuleResource{}
	_ resource.ResourceWithConfigure   = &securityGroupRuleResource{}
	_ resource.ResourceWithImportState = &securityGroupRuleResource{}
)

func NewSecurityGroupRuleResource() resource.Resource {
	return &securityGroupRuleResource{}
}

type securityGroupRuleResource struct {
	client *client.Client
}

type securityGroupRuleResourceModel struct {
	ProjectID       types.String `tfsdk:"project_id"`
	SecurityGroupID types.String `tfsdk:"security_group_id"`
	Position        types.Int64  `tfsdk:"position"`
	Type            types.String `tfsdk:"type"`
	Action          types.String `tfsdk:"action"`
	Proto           types.String `tfsdk:"proto"`
	Source          types.String `tfsdk:"source"`
	Dest            types.String `tfsdk:"dest"`
	SPort           types.String `tfsdk:"sport"`
	DPort           types.String `tfsdk:"dport"`
	Comment         types.String `tfsdk:"comment"`
	Enable          types.Bool   `tfsdk:"enable"`
}

func (r *securityGroupRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_security_group_rule"
}

func (r *securityGroupRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *securityGroupRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One rule of a firewall security group, evaluated by `position` from 0. " +
			"The API addresses a rule only by that position, so rules of one group must be applied in position order (`depends_on`); a parallel apply can shift them. " +
			"Changing `project_id` or `security_group_id` replaces the rule. Import with `<project_id>,<security_group_id>,<position>`.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the project the security group belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"security_group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the security group (`hostd_security_group`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"position": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Zero-based position. Changing it moves the rule.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Direction: `in` or `out`.",
				Validators: []validator.String{
					stringvalidator.OneOf("in", "out"),
				},
			},
			"action": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`ACCEPT` or `DROP`.",
				Validators: []validator.String{
					stringvalidator.OneOf("ACCEPT", "DROP"),
				},
			},
			"proto": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`tcp`, `udp` or `icmp`. Ports are only valid for `tcp` and `udp`.",
				Validators: []validator.String{
					stringvalidator.OneOf("tcp", "udp", "icmp"),
				},
			},
			"source": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "Source address, CIDR (IPv4 no larger than `/26`, IPv6 no larger than `/64`), IPv4 range, or an IP set as `+<pve_ipset_name>`.",
			},
			"dest": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "Destination address, CIDR, IPv4 range, or an IP set as `+<pve_ipset_name>`. Same limits as `source`.",
			},
			"sport": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "Source port, range (`80:85`) or list. TCP and UDP only.",
			},
			"dport": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "Destination port, range (`80:85`) or list. TCP and UDP only.",
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "Free-text comment. Empty by default.",
			},
			"enable": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the rule is enabled. Defaults to true.",
			},
		},
	}
}

func (m securityGroupRuleResourceModel) input(pos *int64) client.SecurityGroupRuleInput {
	in := client.SecurityGroupRuleInput{
		Type:    m.Type.ValueString(),
		Action:  m.Action.ValueString(),
		Proto:   m.Proto.ValueString(),
		Source:  m.Source.ValueString(),
		Dest:    m.Dest.ValueString(),
		SPort:   m.SPort.ValueString(),
		DPort:   m.DPort.ValueString(),
		Comment: m.Comment.ValueString(),
		Enable:  m.Enable.ValueBool(),
	}
	if pos != nil {
		in.Pos = pos
	} else {
		position := m.Position.ValueInt64()
		in.Position = &position
	}
	return in
}

func (m securityGroupRuleResourceModel) sameContent(other securityGroupRuleResourceModel) bool {
	return m.Type.Equal(other.Type) && m.Action.Equal(other.Action) && m.Proto.Equal(other.Proto) &&
		m.Source.Equal(other.Source) && m.Dest.Equal(other.Dest) && m.SPort.Equal(other.SPort) &&
		m.DPort.Equal(other.DPort) && m.Comment.Equal(other.Comment) && m.Enable.Equal(other.Enable)
}

func (r *securityGroupRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data securityGroupRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.CreateSecurityGroupRule(ctx, data.ProjectID.ValueString(), data.SecurityGroupID.ValueString(), data.input(nil))
	if err != nil {
		resp.Diagnostics.AddError("Error creating security group rule", fmt.Sprintf("creating rule at position %d: %s", data.Position.ValueInt64(), err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *securityGroupRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data securityGroupRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := r.client.GetSecurityGroupRule(ctx, data.ProjectID.ValueString(), data.SecurityGroupID.ValueString(), data.Position.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading security group rule", fmt.Sprintf("reading rule at position %d: %s", data.Position.ValueInt64(), err))
		return
	}
	data.Type = types.StringValue(rule.Type)
	data.Action = types.StringValue(rule.Action)
	data.Proto = types.StringValue(rule.Proto)
	data.Source = keepConfigured(data.Source, deref(rule.Source))
	data.Dest = keepConfigured(data.Dest, deref(rule.Dest))
	data.SPort = keepConfigured(data.SPort, deref(rule.SPort))
	data.DPort = keepConfigured(data.DPort, deref(rule.DPort))
	data.Comment = keepConfigured(data.Comment, deref(rule.Comment))
	enabled := true
	if rule.Enable != nil {
		enabled = *rule.Enable
	}
	data.Enable = types.BoolValue(enabled)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *securityGroupRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state securityGroupRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID, groupID := plan.ProjectID.ValueString(), plan.SecurityGroupID.ValueString()
	if !plan.Position.Equal(state.Position) {
		err := r.client.MoveSecurityGroupRule(ctx, projectID, groupID, state.Position.ValueInt64(), plan.Position.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error moving security group rule", fmt.Sprintf("moving rule from %d to %d: %s", state.Position.ValueInt64(), plan.Position.ValueInt64(), err))
			return
		}
	}
	if !plan.sameContent(state) {
		pos := plan.Position.ValueInt64()
		err := r.client.UpdateSecurityGroupRule(ctx, projectID, groupID, plan.input(&pos))
		if err != nil {
			resp.Diagnostics.AddError("Error updating security group rule", fmt.Sprintf("updating rule at position %d: %s", pos, err))
			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *securityGroupRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data securityGroupRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteSecurityGroupRule(ctx, data.ProjectID.ValueString(), data.SecurityGroupID.ValueString(), data.Position.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting security group rule", fmt.Sprintf("deleting rule at position %d: %s", data.Position.ValueInt64(), err))
	}
}

func (r *securityGroupRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectID, rest, ok := strings.Cut(req.ID, ",")
	groupID, rawPos, ok2 := strings.Cut(rest, ",")
	pos, err := strconv.ParseInt(rawPos, 10, 64)
	if !ok || !ok2 || projectID == "" || groupID == "" || err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected <project_id>,<security_group_id>,<position>, got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("security_group_id"), groupID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("position"), pos)...)
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
