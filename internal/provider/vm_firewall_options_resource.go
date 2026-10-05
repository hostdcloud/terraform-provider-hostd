// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	_ resource.Resource                = &vmFirewallOptionsResource{}
	_ resource.ResourceWithConfigure   = &vmFirewallOptionsResource{}
	_ resource.ResourceWithImportState = &vmFirewallOptionsResource{}
)

func NewVMFirewallOptionsResource() resource.Resource {
	return &vmFirewallOptionsResource{}
}

type vmFirewallOptionsResource struct {
	client *client.Client
}

type vmFirewallOptionsResourceModel struct {
	ProjectID types.String `tfsdk:"project_id"`
	VMID      types.Int64  `tfsdk:"vmid"`
	Enable    types.Bool   `tfsdk:"enable"`
	PolicyIn  types.String `tfsdk:"policy_in"`
	PolicyOut types.String `tfsdk:"policy_out"`
}

func (r *vmFirewallOptionsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_firewall_options"
}

func (r *vmFirewallOptionsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *vmFirewallOptionsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Whether a VM firewall is enabled, and its default inbound and outbound policies. " +
			"The firewall filters the public interface. Destroy removes this resource from state only; the VM keeps the settings. " +
			"Import with `<project_id>,<vmid>`.",
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
		},
	}
}

func (r *vmFirewallOptionsResource) apply(ctx context.Context, data vmFirewallOptionsResourceModel) error {
	return r.client.SetVMFirewallOptions(ctx, data.ProjectID.ValueString(), data.VMID.ValueInt64(), data.Enable.ValueBool(), data.PolicyIn.ValueString(), data.PolicyOut.ValueString())
}

func (r *vmFirewallOptionsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data vmFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, data); err != nil {
		resp.Diagnostics.AddError("Error setting VM firewall options", fmt.Sprintf("setting firewall options for VM %d: %s", data.VMID.ValueInt64(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *vmFirewallOptionsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data vmFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	options, err := r.client.GetVMFirewallOptions(ctx, data.ProjectID.ValueString(), data.VMID.ValueInt64())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading VM firewall options", fmt.Sprintf("reading firewall options for VM %d: %s", data.VMID.ValueInt64(), err))
		return
	}
	if options.Enable == nil {
		data.Enable = types.BoolNull()
	} else {
		data.Enable = types.BoolValue(*options.Enable)
	}
	data.PolicyIn = types.StringPointerValue(options.PolicyIn)
	data.PolicyOut = types.StringPointerValue(options.PolicyOut)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *vmFirewallOptionsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data vmFirewallOptionsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, data); err != nil {
		resp.Diagnostics.AddError("Error updating VM firewall options", fmt.Sprintf("updating firewall options for VM %d: %s", data.VMID.ValueInt64(), err))
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *vmFirewallOptionsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *vmFirewallOptionsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectID, rawVMID, ok := strings.Cut(req.ID, ",")
	vmid, err := strconv.ParseInt(rawVMID, 10, 64)
	if !ok || projectID == "" || err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected <project_id>,<vmid>, got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vmid"), vmid)...)
}
