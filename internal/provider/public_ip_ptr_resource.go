// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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

var ptrHostnamePattern = regexp.MustCompile(`^[0-9A-Za-z](?:[0-9A-Za-z-]{0,61}[0-9A-Za-z])?(?:\.[0-9A-Za-z](?:[0-9A-Za-z-]{0,61}[0-9A-Za-z])?)*\.?$`)

var (
	_ resource.Resource                = &publicIPPTRResource{}
	_ resource.ResourceWithConfigure   = &publicIPPTRResource{}
	_ resource.ResourceWithImportState = &publicIPPTRResource{}
)

func NewPublicIPPTRResource() resource.Resource {
	return &publicIPPTRResource{}
}

type publicIPPTRResource struct {
	client *client.Client
}

type publicIPPTRResourceModel struct {
	ProjectID  types.String `tfsdk:"project_id"`
	IP         types.String `tfsdk:"ip"`
	Content    types.String `tfsdk:"content"`
	Comment    types.String `tfsdk:"comment"`
	VMID       types.Int64  `tfsdk:"vmid"`
	Status     types.String `tfsdk:"status"`
	Attachment types.String `tfsdk:"attachment"`
}

func (r *publicIPPTRResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_public_ip_ptr"
}

func (r *publicIPPTRResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *publicIPPTRResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reverse DNS (PTR) for a public IP that is already reserved on a project. " +
			"The address itself is not created here: it comes from a VM or stays reserved in the project. " +
			"Destroy restores the default hostname and leaves the address in place. Import with `<project_id>,<ip>`.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the project the address belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ip": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Public IPv4 or IPv6 address.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"content": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "PTR hostname, for example `vm.example.com`.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 255),
					stringvalidator.RegexMatches(ptrHostnamePattern, "must be a hostname"),
				},
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "Comment stored with the address. Empty by default.",
				Validators: []validator.String{
					stringvalidator.LengthAtMost(120),
				},
			},
			"vmid": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "VMID when the address is attached to a VM.",
			},
			"status": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`attached` or `reserved`.",
			},
			"attachment": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "`vm`, `router` (Cloud Gateway), or empty when the address is only reserved.",
			},
		},
	}
}

func applyPublicIP(data *publicIPPTRResourceModel, ip *client.PublicIP) {
	data.Content = keepConfigured(data.Content, deref(ip.Content))
	data.Comment = keepConfigured(data.Comment, deref(ip.Comment))
	if ip.VMID == nil {
		data.VMID = types.Int64Null()
	} else {
		data.VMID = types.Int64Value(*ip.VMID)
	}
	data.Status = types.StringValue(ip.Status)
	data.Attachment = types.StringPointerValue(ip.Attachment)
}

func (r *publicIPPTRResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data publicIPPTRResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ip, err := r.client.UpdatePublicIP(ctx, data.ProjectID.ValueString(), data.IP.ValueString(), data.Content.ValueString(), data.Comment.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error setting PTR", fmt.Sprintf("setting PTR for %s: %s", data.IP.ValueString(), err))
		return
	}
	applyPublicIP(&data, ip)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *publicIPPTRResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data publicIPPTRResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ip, err := r.client.GetPublicIP(ctx, data.ProjectID.ValueString(), data.IP.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading PTR", fmt.Sprintf("reading %s: %s", data.IP.ValueString(), err))
		return
	}
	applyPublicIP(&data, ip)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *publicIPPTRResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data publicIPPTRResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ip, err := r.client.UpdatePublicIP(ctx, data.ProjectID.ValueString(), data.IP.ValueString(), data.Content.ValueString(), data.Comment.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating PTR", fmt.Sprintf("updating PTR for %s: %s", data.IP.ValueString(), err))
		return
	}
	applyPublicIP(&data, ip)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *publicIPPTRResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data publicIPPTRResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, err := r.client.UpdatePublicIP(ctx, data.ProjectID.ValueString(), data.IP.ValueString(), "", data.Comment.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error clearing PTR", fmt.Sprintf("restoring the default hostname for %s: %s", data.IP.ValueString(), err))
	}
}

func (r *publicIPPTRResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectID, ip, ok := strings.Cut(req.ID, ",")
	if !ok || projectID == "" || ip == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected <project_id>,<ip>, got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip"), ip)...)
}
