// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hostdcloud/terraform-provider-hostd/internal/client"
)

var (
	_ datasource.DataSource              = &osImagesDataSource{}
	_ datasource.DataSourceWithConfigure = &osImagesDataSource{}
)

func NewOSImagesDataSource() datasource.DataSource {
	return &osImagesDataSource{}
}

type osImagesDataSource struct {
	client *client.Client
}

type osImagesDataSourceModel struct {
	ProjectID types.String `tfsdk:"project_id"`
	Names     types.List   `tfsdk:"names"`
}

func (d *osImagesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_os_images"
}

func (d *osImagesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *client.Client, got: %T.", req.ProviderData),
		)
		return
	}
	d.client = c
}

func (d *osImagesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "OS images available for new VMs in a project. The list comes from the project's cluster, so image names cannot be guessed; use one of them as `os_image` of `hostd_vm`.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the project (`hostd_project`).",
			},
			"names": schema.ListAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				MarkdownDescription: "Image names, sorted, e.g. `debian-12-amd64.qcow2`.",
			},
		},
	}
}

func (d *osImagesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data osImagesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	names, err := d.client.ListOSImages(ctx, data.ProjectID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading OS images",
			fmt.Sprintf("reading OS images (project %s): %s", data.ProjectID.ValueString(), err),
		)
		return
	}
	sort.Strings(names)

	var diags diag.Diagnostics
	data.Names, diags = types.ListValueFrom(ctx, types.StringType, names)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
