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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hostdcloud/terraform-provider-hostd/internal/client"
)

var (
	_ resource.Resource                = &vmFirewallRuleResource{}
	_ resource.ResourceWithConfigure   = &vmFirewallRuleResource{}
	_ resource.ResourceWithImportState = &vmFirewallRuleResource{}
	_ resource.ResourceWithModifyPlan  = &vmFirewallRuleResource{}
)

func NewVMFirewallRuleResource() resource.Resource {
	return &vmFirewallRuleResource{}
}

type vmFirewallRuleResource struct {
	client *client.Client
}

type vmFirewallRuleResourceModel struct {
	ProjectID types.String `tfsdk:"project_id"`
	VMID      types.Int64  `tfsdk:"vmid"`
	Position  types.Int64  `tfsdk:"position"`
	Type      types.String `tfsdk:"type"`
	Action    types.String `tfsdk:"action"`
	Proto     types.String `tfsdk:"proto"`
	Source    types.String `tfsdk:"source"`
	Dest      types.String `tfsdk:"dest"`
	SPort     types.String `tfsdk:"sport"`
	DPort     types.String `tfsdk:"dport"`
	Comment   types.String `tfsdk:"comment"`
	Enable    types.Bool   `tfsdk:"enable"`
}

func (r *vmFirewallRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_firewall_rule"
}

func (r *vmFirewallRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *vmFirewallRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One VM firewall rule, or an attached security group. The firewall filters the public interface. " +
			"Rules are evaluated by `position` from 0; rules of one VM must be applied in that order (`depends_on`). " +
			"`type = group` attaches a security group: `action` is its `pve_group_name`. " +
			"Import with `<project_id>,<vmid>,<position>`.",
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
			"position": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Zero-based position. Changing it moves the rule.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`in` or `out` for a packet filter, or `group` to attach a security group.",
				Validators: []validator.String{
					stringvalidator.OneOf("in", "out", "group"),
				},
			},
			"action": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "`ACCEPT` or `DROP` for a packet filter. For `type = group`, the security group's `pve_group_name`.",
			},
			"proto": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "`tcp`, `udp` or `icmp` for a packet filter. Leave empty for a security group.",
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

func (m vmFirewallRuleResourceModel) invalid() string {
	switch m.Type.ValueString() {
	case "group":
		if m.Proto.ValueString() != "" || m.Source.ValueString() != "" || m.Dest.ValueString() != "" || m.SPort.ValueString() != "" || m.DPort.ValueString() != "" {
			return "A group attachment uses action (the group's pve_group_name), comment and enable."
		}
	case "in", "out":
		if m.Action.ValueString() != "ACCEPT" && m.Action.ValueString() != "DROP" {
			return "action must be ACCEPT or DROP."
		}
		switch m.Proto.ValueString() {
		case "tcp", "udp", "icmp":
		default:
			return "proto must be tcp, udp or icmp."
		}
	}
	return ""
}

func (m vmFirewallRuleResourceModel) input(pos *int64) client.VMFirewallRuleInput {
	in := client.VMFirewallRuleInput{
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

func (m vmFirewallRuleResourceModel) sameContent(other vmFirewallRuleResourceModel) bool {
	return m.Type.Equal(other.Type) && m.Action.Equal(other.Action) && m.Proto.Equal(other.Proto) &&
		m.Source.Equal(other.Source) && m.Dest.Equal(other.Dest) && m.SPort.Equal(other.SPort) &&
		m.DPort.Equal(other.DPort) && m.Comment.Equal(other.Comment) && m.Enable.Equal(other.Enable)
}

func (r *vmFirewallRuleResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var plan vmFirewallRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if msg := plan.invalid(); msg != "" {
		resp.Diagnostics.AddError("Invalid VM firewall rule", msg)
	}
}

func (r *vmFirewallRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data vmFirewallRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if msg := data.invalid(); msg != "" {
		resp.Diagnostics.AddError("Invalid VM firewall rule", msg)
		return
	}

	err := r.client.CreateVMFirewallRule(ctx, data.ProjectID.ValueString(), data.VMID.ValueInt64(), data.input(nil))
	if err != nil {
		resp.Diagnostics.AddError("Error creating VM firewall rule", fmt.Sprintf("creating rule at position %d on VM %d: %s", data.Position.ValueInt64(), data.VMID.ValueInt64(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *vmFirewallRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data vmFirewallRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	rule, err := r.client.GetVMFirewallRule(ctx, data.ProjectID.ValueString(), data.VMID.ValueInt64(), data.Position.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading VM firewall rule", fmt.Sprintf("reading rule at position %d on VM %d: %s", data.Position.ValueInt64(), data.VMID.ValueInt64(), err))
		return
	}
	data.Type = types.StringValue(rule.Type)
	data.Action = types.StringValue(deref(rule.Action))
	data.Proto = keepConfigured(data.Proto, deref(rule.Proto))
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

func (r *vmFirewallRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vmFirewallRuleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if msg := plan.invalid(); msg != "" {
		resp.Diagnostics.AddError("Invalid VM firewall rule", msg)
		return
	}

	projectID, vmid := plan.ProjectID.ValueString(), plan.VMID.ValueInt64()
	if !plan.Position.Equal(state.Position) {
		err := r.client.MoveVMFirewallRule(ctx, projectID, vmid, state.Position.ValueInt64(), plan.Position.ValueInt64())
		if err != nil {
			resp.Diagnostics.AddError("Error moving VM firewall rule", fmt.Sprintf("moving rule from %d to %d on VM %d: %s", state.Position.ValueInt64(), plan.Position.ValueInt64(), vmid, err))
			return
		}
	}
	if !plan.sameContent(state) {
		pos := plan.Position.ValueInt64()
		err := r.client.UpdateVMFirewallRule(ctx, projectID, vmid, plan.input(&pos))
		if err != nil {
			resp.Diagnostics.AddError("Error updating VM firewall rule", fmt.Sprintf("updating rule at position %d on VM %d: %s", pos, vmid, err))
			return
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vmFirewallRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data vmFirewallRuleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteVMFirewallRule(ctx, data.ProjectID.ValueString(), data.VMID.ValueInt64(), data.Position.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting VM firewall rule", fmt.Sprintf("deleting rule at position %d on VM %d: %s", data.Position.ValueInt64(), data.VMID.ValueInt64(), err))
	}
}

func (r *vmFirewallRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectID, rest, ok := strings.Cut(req.ID, ",")
	rawVMID, rawPos, ok2 := strings.Cut(rest, ",")
	vmid, errVM := strconv.ParseInt(rawVMID, 10, 64)
	pos, errPos := strconv.ParseInt(rawPos, 10, 64)
	if !ok || !ok2 || projectID == "" || errVM != nil || errPos != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected <project_id>,<vmid>,<position>, got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vmid"), vmid)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("position"), pos)...)
}
