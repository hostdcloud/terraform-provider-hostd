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
	_ datasource.DataSource              = &privateNetworksDataSource{}
	_ datasource.DataSourceWithConfigure = &privateNetworksDataSource{}
)

func NewPrivateNetworksDataSource() datasource.DataSource {
	return &privateNetworksDataSource{}
}

type privateNetworksDataSource struct {
	client *client.Client
}

type privateNetworksDataSourceModel struct {
	ProjectID types.String          `tfsdk:"project_id"`
	Networks  []privateNetworkModel `tfsdk:"networks"`
}

type privateNetworkModel struct {
	VNet          types.String `tfsdk:"vnet"`
	Type          types.String `tfsdk:"type"`
	LANIdentifier types.Int64  `tfsdk:"lan_identifier"`
	CIDR          types.String `tfsdk:"cidr"`
	Gateway       types.String `tfsdk:"gateway"`
	HasRouter     types.Bool   `tfsdk:"has_router"`
	FreeIPs       types.List   `tfsdk:"free_ips"`
}

func (d *privateNetworksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_private_networks"
}

func (d *privateNetworksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *privateNetworksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Private networks of a project. `free_ips` lists addresses that `lan_interfaces` can take with `mode = select`. Create the networks in the portal first.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the project (`hostd_project`).",
			},
			"networks": schema.ListNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Private networks in the project.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"vnet": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Network name. Use it as `vnet` in `lan_interfaces`.",
						},
						"type": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Network type. `tag` is an internal VLAN.",
						},
						"lan_identifier": schema.Int64Attribute{
							Computed:            true,
							MarkdownDescription: "NIC index on a VM: `1` is `net1`.",
						},
						"cidr": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "IPv4 subnet.",
						},
						"gateway": schema.StringAttribute{
							Computed:            true,
							MarkdownDescription: "Gateway address of the subnet.",
						},
						"has_router": schema.BoolAttribute{
							Computed:            true,
							MarkdownDescription: "Whether a Cloud Gateway is attached.",
						},
						"free_ips": schema.ListAttribute{
							ElementType:         types.StringType,
							Computed:            true,
							MarkdownDescription: "Host addresses that no VM or Cloud Gateway uses.",
						},
					},
				},
			},
		},
	}
}

func (d *privateNetworksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data privateNetworksDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID := data.ProjectID.ValueString()
	networks, err := d.client.ListPrivateNetworks(ctx, projectID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading private networks", fmt.Sprintf("listing private networks: %s", err))
		return
	}
	data.Networks = make([]privateNetworkModel, 0, len(networks))
	for _, network := range networks {
		item := privateNetworkModel{
			VNet:          types.StringPointerValue(network.VNet),
			Type:          types.StringValue(network.Type),
			LANIdentifier: types.Int64Value(network.LANIdentifier),
			CIDR:          types.StringPointerValue(network.CIDR),
			Gateway:       types.StringPointerValue(network.Gateway),
			HasRouter:     types.BoolValue(network.HasRouter),
		}
		free := []string{}
		if network.VNet != nil && *network.VNet != "" {
			ips, err := d.client.ListPrivateNetworkIPs(ctx, projectID, *network.VNet)
			if err != nil {
				resp.Diagnostics.AddError("Error reading private network addresses", fmt.Sprintf("listing addresses of %s: %s", *network.VNet, err))
				return
			}
			for _, ip := range ips {
				if ip.Free {
					free = append(free, ip.IP)
				}
			}
		}
		freeIPs, diags := types.ListValueFrom(ctx, types.StringType, free)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		item.FreeIPs = freeIPs
		data.Networks = append(data.Networks, item)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
