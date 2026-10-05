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

var (
	_ resource.Resource                = &ipSetResource{}
	_ resource.ResourceWithConfigure   = &ipSetResource{}
	_ resource.ResourceWithImportState = &ipSetResource{}
)

var ipSetNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,14}$`)

func NewIPSetResource() resource.Resource {
	return &ipSetResource{}
}

type ipSetResource struct {
	client *client.Client
}

type ipSetResourceModel struct {
	ID           types.String `tfsdk:"id"`
	ProjectID    types.String `tfsdk:"project_id"`
	Name         types.String `tfsdk:"name"`
	Comment      types.String `tfsdk:"comment"`
	PVEIPSetName types.String `tfsdk:"pve_ipset_name"`
}

func (r *ipSetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ip_set"
}

func (r *ipSetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ipSetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A project firewall IP set: a named list of addresses that rules reference as `+pve_ipset_name`. " +
			"Deleting the set also removes VM and security-group rules that use it. Import with `<project_id>,<id>`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "IP set UUID.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the project (`hostd_project`) the set belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Name: a lowercase letter, then up to 14 letters, digits, hyphens or underscores. The API stores it in lower case.",
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
			"pve_ipset_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Identifier firewall rules use, prefixed with `+`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ipSetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ipSetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	set, err := r.client.CreateIPSet(ctx, data.ProjectID.ValueString(), data.Name.ValueString(), data.Comment.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating IP set", fmt.Sprintf("creating IP set (%s): %s", data.Name.ValueString(), err))
		return
	}
	data.ID = types.StringValue(set.UUID)
	data.Name = types.StringValue(set.DisplayName)
	data.Comment = types.StringValue(set.Comment)
	data.PVEIPSetName = types.StringValue(set.PVEIPSetName)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ipSetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ipSetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	set, err := r.client.GetIPSet(ctx, data.ProjectID.ValueString(), data.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading IP set", fmt.Sprintf("reading IP set (%s): %s", data.ID.ValueString(), err))
		return
	}
	data.Name = keepConfigured(data.Name, set.DisplayName)
	data.Comment = keepConfigured(data.Comment, set.Comment)
	data.PVEIPSetName = types.StringValue(set.PVEIPSetName)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ipSetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ipSetResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	set, err := r.client.UpdateIPSet(ctx, data.ProjectID.ValueString(), data.ID.ValueString(), data.Name.ValueString(), data.Comment.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating IP set", fmt.Sprintf("updating IP set (%s): %s", data.ID.ValueString(), err))
		return
	}
	data.Name = types.StringValue(set.DisplayName)
	data.Comment = types.StringValue(set.Comment)
	data.PVEIPSetName = types.StringValue(set.PVEIPSetName)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ipSetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ipSetResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteIPSet(ctx, data.ProjectID.ValueString(), data.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting IP set", fmt.Sprintf("deleting IP set (%s): %s", data.ID.ValueString(), err))
	}
}

func (r *ipSetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectID, id, ok := strings.Cut(req.ID, ",")
	if !ok || projectID == "" || id == "" {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected <project_id>,<id>, got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
