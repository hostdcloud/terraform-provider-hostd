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
	_ datasource.DataSource              = &ipAddressesDataSource{}
	_ datasource.DataSourceWithConfigure = &ipAddressesDataSource{}
)

func NewIPAddressesDataSource() datasource.DataSource {
	return &ipAddressesDataSource{}
}

type ipAddressesDataSource struct {
	client *client.Client
}

type ipAddressesDataSourceModel struct {
	Addresses []ipAddressModel `tfsdk:"addresses"`
}

type ipAddressModel struct {
	IP           types.String `tfsdk:"ip"`
	Content      types.String `tfsdk:"content"`
	Comment      types.String `tfsdk:"comment"`
	CanUpdatePTR types.Bool   `tfsdk:"can_update_ptr"`
	ServiceKind  types.String `tfsdk:"service_kind"`
	ServiceName  types.String `tfsdk:"service_name"`
	ServiceSID   types.String `tfsdk:"service_sid"`
	ServiceUUID  types.String `tfsdk:"service_uuid"`
	Status       types.String `tfsdk:"status"`
	CanRelease   types.Bool   `tfsdk:"can_release"`
	VM           *ipAddressVM `tfsdk:"vm"`
}

type ipAddressVM struct {
	VMID types.Int64  `tfsdk:"vmid"`
	Name types.String `tfsdk:"name"`
}

func (d *ipAddressesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ip_addresses"
}

func (d *ipAddressesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ipAddressesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Public addresses on the account: dedicated servers, VPS and cloud projects, with the current PTR and whether it can be changed.",
		Attributes: map[string]schema.Attribute{
			"addresses": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Addresses assigned to your services.",
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
						"can_update_ptr": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether this key may change the PTR.",
						},
						"service_kind": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "`ds`, `vps` or `cloud`.",
						},
						"service_name": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Name of the service the address is attached to.",
						},
						"service_sid": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Service SID for dedicated servers and VPS.",
						},
						"service_uuid": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "UUID of the dedicated server, VPS or cloud project.",
						},
						"status": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "`attached` or `reserved`.",
						},
						"can_release": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether the address may be released to the pool.",
						},
						"vm": schema.SingleNestedAttribute{
							Computed:            true,
							MarkdownDescription: "Cloud VM the address is attached to.",
							Attributes: map[string]schema.Attribute{
								"vmid": schema.Int64Attribute{
									Computed:            true,
									MarkdownDescription: "VMID.",
								},
								"name": schema.StringAttribute{
									Computed:            true,
									MarkdownDescription: "VM name.",
								},
							},
						},
					},
				},
			},
		},
	}
}

func (d *ipAddressesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ipAddressesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ips, err := d.client.ListAccountIPs(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Error reading IP addresses", fmt.Sprintf("listing IP addresses: %s", err))
		return
	}
	data.Addresses = make([]ipAddressModel, 0, len(ips))
	for _, ip := range ips {
		item := ipAddressModel{
			IP:           types.StringPointerValue(ip.IP),
			Content:      types.StringValue(deref(ip.Content)),
			Comment:      types.StringValue(deref(ip.Comment)),
			CanUpdatePTR: types.BoolValue(ip.CanUpdatePTR),
			ServiceKind:  types.StringPointerValue(ip.ServiceKind),
			ServiceName:  types.StringValue(ip.ServiceName),
			ServiceSID:   types.StringPointerValue(ip.ServiceSID),
			ServiceUUID:  types.StringPointerValue(ip.ServiceUUID),
			Status:       types.StringValue(ip.Status),
			CanRelease:   types.BoolValue(ip.CanRelease),
		}
		if ip.VM != nil {
			item.VM = &ipAddressVM{
				VMID: types.Int64Value(ip.VM.VMID),
				Name: types.StringPointerValue(ip.VM.Name),
			}
		}
		data.Addresses = append(data.Addresses, item)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
