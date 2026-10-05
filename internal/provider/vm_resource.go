// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/rand"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hostdcloud/terraform-provider-hostd/internal/client"
)

var (
	_ resource.Resource                = &vmResource{}
	_ resource.ResourceWithConfigure   = &vmResource{}
	_ resource.ResourceWithImportState = &vmResource{}
)

func NewVMResource() resource.Resource {
	return &vmResource{}
}

type vmResource struct {
	client *client.Client
}

type vmResourceModel struct {
	ProjectID types.String `tfsdk:"project_id"`
	VMID      types.Int64  `tfsdk:"vmid"`
	Hostname  types.String `tfsdk:"hostname"`
	OSImage   types.String `tfsdk:"os_image"`
	Username  types.String `tfsdk:"username"`
	SSHKeyIDs types.Set    `tfsdk:"ssh_key_ids"`
	Cores     types.Int64  `tfsdk:"cores"`
	RAMGB     types.Int64  `tfsdk:"ram_gb"`
	OSDiskGB  types.Int64  `tfsdk:"os_disk_gb"`
	CPUMode   types.String `tfsdk:"cpu_mode"`
	WANMode   types.String `tfsdk:"wan_mode"`
	LANVNets  types.Set    `tfsdk:"lan_vnets"`
	IPv4      types.String `tfsdk:"ipv4"`
	IPv6      types.String `tfsdk:"ipv6"`
}

func (r *vmResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm"
}

func (r *vmResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *vmResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Public Cloud virtual machine. Creation waits until the VM is running; destroying hard-stops it first. " +
			"CPU, RAM, disk, CPU class and networking can be changed in place (the API only allows lowering CPU or RAM on a stopped VM, 24 hours after create or the last increase). " +
			"Import with `<project_id>/<vmid>`; `os_image`, `ssh_key_ids` and `username` are create-time inputs the API does not return. After an import, set `os_image` and `ssh_key_ids` in the configuration (they are recorded without replacing the VM) and omit `username`.",
		Attributes: map[string]schema.Attribute{
			"project_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "UUID of the project (`hostd_project`) the VM belongs to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"vmid": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "VM identifier (VMID) assigned by the API.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"hostname": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "DNS-style hostname (letters, digits, hyphens and dots; at most 64 characters).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"os_image": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Image name available in the project, e.g. `debian-12-amd64.qcow2`. List the available names with the `hostd_os_images` data source.",
				PlanModifiers: []planmodifier.String{
					// Empty after import: setting it then must not replace the VM.
					stringplanmodifier.RequiresReplaceIf(
						func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
							resp.RequiresReplace = !req.StateValue.IsNull()
						},
						"Changing the image replaces the VM.",
						"Changing the image replaces the VM.",
					),
				},
			},
			"username": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Cloud-init username. Defaults to the OS image's default user.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ssh_key_ids": schema.SetAttribute{
				ElementType:         types.Int64Type,
				Required:            true,
				MarkdownDescription: "IDs of SSH keys (`hostd_ssh_key`) to install, from one to five. The VM is reached with these keys: the provider sets a random cloud-init password and does not expose it.",
				Validators: []validator.Set{
					setvalidator.SizeBetween(1, 5),
				},
				PlanModifiers: []planmodifier.Set{
					// Empty after import: setting it then must not replace the VM.
					setplanmodifier.RequiresReplaceIf(
						func(_ context.Context, req planmodifier.SetRequest, resp *setplanmodifier.RequiresReplaceIfFuncResponse) {
							resp.RequiresReplace = !req.StateValue.IsNull()
						},
						"Changing the SSH keys replaces the VM.",
						"Changing the SSH keys replaces the VM.",
					),
				},
			},
			"cores": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "vCPU count (even numbers from 2), within the account CPU quotas.",
			},
			"ram_gb": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "RAM in GiB; between 1 and 4 GiB per vCPU.",
			},
			"os_disk_gb": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "OS disk size in GiB, in steps of 20 GiB; between 10 and 100 GiB per vCPU. It cannot shrink.",
			},
			"cpu_mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("regular"),
				MarkdownDescription: "CPU class: `light` or `regular` (default).",
			},
			"wan_mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("dual"),
				MarkdownDescription: "Public network: `off`, `ipv4`, `ipv6` or `dual` (default). With `off`, attach at least one private network. Switching between `ipv4`, `ipv6` and `dual` requires going through `off`.",
			},
			"lan_vnets": schema.SetAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Default:             setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				MarkdownDescription: "Names (VNet) of private networks to attach; each gets a free address.",
			},
			"ipv4": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public IPv4 address, empty when none is attached.",
			},
			"ipv6": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public IPv6 address, empty when none is attached.",
			},
		},
	}
}

func (r *vmResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data vmResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	in := client.CreateVMInput{
		Hostname: data.Hostname.ValueString(),
		Username: data.Username.ValueString(),
		Password: rand.Text(), // required by the API; VMs are reached with SSH keys
		Cores:    data.Cores.ValueInt64(),
		RAMGB:    data.RAMGB.ValueInt64(),
		OSDiskGB: data.OSDiskGB.ValueInt64(),
		OSImage:  data.OSImage.ValueString(),
		CPUMode:  data.CPUMode.ValueString(),
		WANMode:  data.WANMode.ValueString(),
	}
	resp.Diagnostics.Append(data.SSHKeyIDs.ElementsAs(ctx, &in.SSHKeyIDs, false)...)
	resp.Diagnostics.Append(data.LANVNets.ElementsAs(ctx, &in.LANVNets, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID := data.ProjectID.ValueString()
	vmid, err := r.client.CreateVM(ctx, projectID, in)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating VM",
			fmt.Sprintf("creating VM (%s): %s", in.Hostname, err),
		)
		return
	}

	// Save the VM before waiting, so a failed wait leaves it tracked (tainted) instead of orphaned.
	data.VMID = types.Int64Value(vmid)
	data.IPv4, data.IPv6 = types.StringNull(), types.StringNull()
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)

	if err := r.client.WaitVMStatus(ctx, projectID, vmid, "running"); err != nil {
		resp.Diagnostics.AddError(
			"Error waiting for VM",
			fmt.Sprintf("waiting for VM %d to run: %s", vmid, err),
		)
		return
	}
	cfg, err := r.client.GetVMConfig(ctx, projectID, vmid)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading VM",
			fmt.Sprintf("reading VM %d: %s", vmid, err),
		)
		return
	}
	data.setAddresses(cfg)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *vmResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data vmResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID, vmid := data.ProjectID.ValueString(), data.VMID.ValueInt64()
	vm, err := r.client.GetVM(ctx, projectID, vmid)
	if err == nil {
		var cfg *client.VMConfig
		if cfg, err = r.client.GetVMConfig(ctx, projectID, vmid); err == nil {
			resp.Diagnostics.Append(data.setFromAPI(ctx, vm, cfg)...)
			if !resp.Diagnostics.HasError() {
				resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
			}
			return
		}
	}
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.AddError(
		"Error reading VM",
		fmt.Sprintf("reading VM %d: %s", vmid, err),
	)
}

func (r *vmResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vmResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	planInput, diags := plan.upgradeInput(ctx)
	resp.Diagnostics.Append(diags...)
	stateInput, diags := state.upgradeInput(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID, vmid := plan.ProjectID.ValueString(), plan.VMID.ValueInt64()
	if !reflect.DeepEqual(planInput, stateInput) {
		if err := r.client.UpgradeVM(ctx, projectID, vmid, planInput); err != nil {
			resp.Diagnostics.AddError(
				"Error updating VM",
				fmt.Sprintf("updating VM %d: %s", vmid, err),
			)
			return
		}
	}

	cfg, err := r.client.GetVMConfig(ctx, projectID, vmid)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading VM",
			fmt.Sprintf("reading VM %d: %s", vmid, err),
		)
		return
	}
	plan.setAddresses(cfg)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vmResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data vmResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DestroyVM(ctx, data.ProjectID.ValueString(), data.VMID.ValueInt64())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError(
			"Error deleting VM",
			fmt.Sprintf("deleting VM %d: %s", data.VMID.ValueInt64(), err),
		)
	}
}

func (r *vmResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	projectID, rawVMID, ok := strings.Cut(req.ID, "/")
	vmid, err := strconv.ParseInt(rawVMID, 10, 64)
	if !ok || err != nil {
		resp.Diagnostics.AddError("Invalid import ID", fmt.Sprintf("Expected <project_id>/<vmid>, got %q.", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vmid"), vmid)...)
}

// upgradeInput is the resizable part of the model in the shape the API expects.
func (m vmResourceModel) upgradeInput(ctx context.Context) (client.UpgradeVMInput, diag.Diagnostics) {
	var vnets []string
	diags := m.LANVNets.ElementsAs(ctx, &vnets, false)
	sort.Strings(vnets)
	lan := make([]client.LANInterface, 0, len(vnets))
	for _, vnet := range vnets {
		lan = append(lan, client.LANInterface{VNet: vnet, Mode: "auto"})
	}
	return client.UpgradeVMInput{
		Cores:         m.Cores.ValueInt64(),
		RAMGB:         m.RAMGB.ValueInt64(),
		OSDiskGB:      m.OSDiskGB.ValueInt64(),
		CPUMode:       m.CPUMode.ValueString(),
		WANMode:       m.WANMode.ValueString(),
		LANInterfaces: lan,
	}, diags
}

func (m *vmResourceModel) setAddresses(cfg *client.VMConfig) {
	m.IPv4, m.IPv6 = types.StringNull(), types.StringNull()
	if cfg.WAN != nil {
		m.IPv4 = types.StringPointerValue(cfg.WAN.IPv4)
		m.IPv6 = types.StringPointerValue(cfg.WAN.IPv6)
	}
}

func (m *vmResourceModel) setFromAPI(ctx context.Context, vm *client.VM, cfg *client.VMConfig) diag.Diagnostics {
	m.Hostname = types.StringValue(cfg.Name)
	m.Cores = types.Int64Value(cfg.Cores)
	m.RAMGB = types.Int64Value(cfg.RAMGB)
	if cfg.OSDiskGB != nil {
		m.OSDiskGB = types.Int64Value(*cfg.OSDiskGB)
	}
	m.CPUMode = types.StringValue(vm.CPUMode)
	m.WANMode = types.StringValue(wanMode(cfg.WAN))
	m.setAddresses(cfg)

	vnets := make([]string, 0, len(cfg.LANInterfaces))
	for _, lan := range cfg.LANInterfaces {
		vnets = append(vnets, lan.VNet)
	}
	var diags diag.Diagnostics
	m.LANVNets, diags = types.SetValueFrom(ctx, types.StringType, vnets)
	return diags
}

func wanMode(wan *client.VMWAN) string {
	if wan == nil {
		return "off"
	}
	switch {
	case wan.IPv4 != nil && wan.IPv6 != nil:
		return "dual"
	case wan.IPv4 != nil:
		return "ipv4"
	case wan.IPv6 != nil:
		return "ipv6"
	}
	return "off"
}
