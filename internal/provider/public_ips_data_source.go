// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hostdcloud/terraform-provider-hostd/internal/client"
)

var (
	_ datasource.DataSource              = &publicIPsDataSource{}
	_ datasource.DataSourceWithConfigure = &publicIPsDataSource{}
)

func NewPublicIPsDataSource() datasource.DataSource {
	return &publicIPsDataSource{}
}

type publicIPsDataSource struct {
	client *client.Client
}

type publicIPsDataSourceModel struct {
	ProjectID types.String        `tfsdk:"project_id"`
	IPs       []publicIPItemModel `tfsdk:"ips"`
}

type publicIPItemModel struct {
	IP         types.String `tfsdk:"ip"`
	Content    types.String `tfsdk:"content"`
	Comment    types.String `tfsdk:"comment"`
	VMID       types.Int64  `tfsdk:"vmid"`
	Status     types.String `tfsdk:"status"`
	Attachment types.String `tfsdk:"attachment"`
}

func (d *publicIPsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_public_ips"
}

func (d *publicIPsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *publicIPsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Public IPv4 and IPv6 addresses reserved on a project, with their PTR hostname and whether each address is in use.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the project (`hostd_project`).",
			},
			"ips": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Addresses reserved on the project.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"ip": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Public IPv4 or IPv6 address.",
						},
						"content": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "PTR hostname.",
						},
						"comment": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Comment stored with the address.",
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
							MarkdownDescription: "`vm`, `router`, or empty when the address is only reserved.",
						},
					},
				},
			},
		},
	}
}

func (d *publicIPsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data publicIPsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ips, err := d.client.ListPublicIPs(ctx, data.ProjectID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading public IPs", fmt.Sprintf("listing public IPs: %s", err))
		return
	}
	data.IPs = make([]publicIPItemModel, 0, len(ips))
	for _, ip := range ips {
		item := publicIPItemModel{
			IP:         types.StringValue(ip.IP),
			Content:    types.StringValue(deref(ip.Content)),
			Comment:    types.StringValue(deref(ip.Comment)),
			Status:     types.StringValue(ip.Status),
			Attachment: types.StringPointerValue(ip.Attachment),
		}
		if ip.VMID == nil {
			item.VMID = types.Int64Null()
		} else {
			item.VMID = types.Int64Value(*ip.VMID)
		}
		data.IPs = append(data.IPs, item)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
