// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hostdcloud/terraform-provider-hostd/internal/client"
)

var (
	_ resource.Resource                = &ipSetMemberResource{}
	_ resource.ResourceWithConfigure   = &ipSetMemberResource{}
	_ resource.ResourceWithImportState = &ipSetMemberResource{}
)

func NewIPSetMemberResource() resource.Resource {
	return &ipSetMemberResource{}
}

type ipSetMemberResource struct {
	client *client.Client
}

type ipSetMemberResourceModel struct {
	ProjectID types.String `tfsdk:"project_id"`
	IPSetID   types.String `tfsdk:"ip_set_id"`
	CIDR      types.String `tfsdk:"cidr"`
	Comment   types.String `tfsdk:"comment"`
	NoMatch   types.Bool   `tfsdk:"nomatch"`
}

func (r *ipSetMemberResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ip_set_member"
}

func (r *ipSetMemberResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ipSetMemberResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One address or network in a firewall IP set. IPv4 up to `/24`, IPv6 up to `/64`. " +
			"Changing `project_id`, `ip_set_id` or `cidr` replaces the member. Import with `<project_id>,<ip_set_id>,<cidr>`.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the project the IP set belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ip_set_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the IP set (`hostd_ip_set`).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cidr": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "IPv4 or IPv6 address or CIDR. An IPv4 network can be no larger than `/24`, an IPv6 network no larger than `/64`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				MarkdownDescription: "Free-text comment. Empty by default.",
			},
			"nomatch": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "When true, the entry is excluded from the set instead of included.",
			},
		},
	}
}

func (m ipSetMemberResourceModel) member() client.IPSetMember {
	return client.IPSetMember{
		CIDR:    m.CIDR.ValueString(),
		Comment: m.Comment.ValueString(),
		NoMatch: m.NoMatch.ValueBool(),
	}
}

func (r *ipSetMemberResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ipSetMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.CreateIPSetMember(ctx, data.ProjectID.ValueString(), data.IPSetID.ValueString(), data.member())
	if err != nil {
		resp.Diagnostics.AddError("Error creating IP set member", fmt.Sprintf("adding %s: %s", data.CIDR.ValueString(), err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ipSetMemberResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ipSetMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	member, err := r.client.GetIPSetMember(ctx, data.ProjectID.ValueString(), data.IPSetID.ValueString(), data.CIDR.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading IP set member", fmt.Sprintf("reading %s: %s", data.CIDR.ValueString(), err))
		return
	}
	data.Comment = keepConfigured(data.Comment, member.Comment)
	data.NoMatch = types.BoolValue(member.NoMatch)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ipSetMemberResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ipSetMemberResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.UpdateIPSetMember(ctx, data.ProjectID.ValueString(), data.IPSetID.ValueString(), data.member())
	if err != nil {
		resp.Diagnostics.AddError("Error updating IP set member", fmt.Sprintf("updating %s: %s", data.CIDR.ValueString(), err))
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ipSetMemberResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ipSetMemberResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteIPSetMember(ctx, data.ProjectID.ValueString(), data.IPSetID.ValueString(), data.CIDR.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting IP set member", fmt.Sprintf("removing %s: %s", data.CIDR.ValueString(), err))
	}
}

func (r *ipSetMemberResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectID, rest, ok := strings.Cut(req.ID, ",")
	ipSetID, cidr, ok2 := strings.Cut(rest, ",")
	if !ok || !ok2 || projectID == "" || ipSetID == "" || cidr == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected <project_id>,<ip_set_id>,<cidr>, got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("ip_set_id"), ipSetID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("cidr"), cidr)...)
}
