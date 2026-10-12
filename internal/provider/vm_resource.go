// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"crypto/rand"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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

var lanInterfaceAttrTypes = map[string]attr.Type{
	"vnet":       types.StringType,
	"mode":       types.StringType,
	"ip":         types.StringType,
	"is_gateway": types.BoolType,
}

type lanInterfaceModel struct {
	VNet      types.String `tfsdk:"vnet"`
	Mode      types.String `tfsdk:"mode"`
	IP        types.String `tfsdk:"ip"`
	IsGateway types.Bool   `tfsdk:"is_gateway"`
}

const (
	vmCreateTimeout = 10 * time.Minute
	vmUpdateTimeout = 15 * time.Minute
	vmDeleteTimeout = 10 * time.Minute
)

var (
	_ resource.Resource                   = &vmResource{}
	_ resource.ResourceWithConfigure      = &vmResource{}
	_ resource.ResourceWithImportState    = &vmResource{}
	_ resource.ResourceWithModifyPlan     = &vmResource{}
	_ resource.ResourceWithValidateConfig = &vmResource{}
)

var hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?)*$`)

func NewVMResource() resource.Resource {
	return &vmResource{}
}

type vmResource struct {
	client *client.Client
}

type vmResourceModel struct {
	ProjectID       types.String   `tfsdk:"project_id"`
	VMID            types.Int64    `tfsdk:"vmid"`
	Hostname        types.String   `tfsdk:"hostname"`
	OSImage         types.String   `tfsdk:"os_image"`
	Username        types.String   `tfsdk:"username"`
	SSHKeyIDs       types.Set      `tfsdk:"ssh_key_ids"`
	Cores           types.Int64    `tfsdk:"cores"`
	RAMGB           types.Int64    `tfsdk:"ram_gb"`
	OSDiskGB        types.Int64    `tfsdk:"os_disk_gb"`
	CPUMode         types.String   `tfsdk:"cpu_mode"`
	WANMode         types.String   `tfsdk:"wan_mode"`
	WANIP           types.String   `tfsdk:"wan_ip"`
	WANIPv6         types.String   `tfsdk:"wan_ipv6"`
	RetainPublicIPs types.Bool     `tfsdk:"retain_public_ips"`
	LANInterfaces   types.List     `tfsdk:"lan_interfaces"`
	PowerState      types.String   `tfsdk:"power_state"`
	IPv4            types.String   `tfsdk:"ipv4"`
	IPv6            types.String   `tfsdk:"ipv6"`
	Timeouts        timeouts.Value `tfsdk:"timeouts"`
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

func (r *vmResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A Public Cloud virtual machine. Creation waits until the VM is running. If that call is interrupted, a VM with the same hostname that appeared during the call is kept instead of creating another; an existing VM with that hostname is never adopted. " +
			"Destroying stops the VM, deletes its disks, and deletes all of its backups. Public addresses return to the pool unless `retain_public_ips` is true (a reserved IPv4 stays billed). " +
			"Changing `project_id`, `hostname`, `os_image`, `username`, `wan_ip`, or `wan_ipv6` replaces the VM. For a production VM set `lifecycle { prevent_destroy = true }`. " +
			"CPU, RAM, disk, CPU class and networking can be changed in place. Lowering CPU or RAM requires `power_state` `stopped` and is refused until 24 hours after create or the last increase. " +
			"Import with `<project_id>/<vmid>`; `os_image`, `ssh_key_ids` and `username` are create-time inputs the API does not return. After an import, set `os_image` and `ssh_key_ids` in the configuration (they are recorded without replacing the VM) and omit `username`. " +
			"A VM that was paused outside Terraform is resumed or stopped to match `power_state`.",
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
				Validators: []validator.String{
					stringvalidator.LengthAtMost(64),
					stringvalidator.RegexMatches(hostnamePattern, "must be a DNS hostname: letters, digits, hyphens and dots, no underscores"),
				},
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
				ElementType: types.Int64Type,
				Required:    true,
				MarkdownDescription: "IDs of SSH keys (`hostd_ssh_key`) to install, from one to five. The VM is reached with these keys: the provider sets a random cloud-init password and does not expose it. " +
					"Keys are installed only when the VM is created. A later change is recorded without touching the VM, so rotating or renaming a `hostd_ssh_key` never replaces it; update `~/.ssh/authorized_keys` on the guest yourself.",
				Validators: []validator.Set{
					setvalidator.SizeBetween(1, 5),
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
				Validators: []validator.String{
					stringvalidator.OneOf("light", "regular"),
				},
			},
			"power_state": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("running"),
				MarkdownDescription: "`running` (default) or `stopped`. `stopped` shuts the guest down. Set it to `stopped` to lower CPU or RAM. The API also refuses that downgrade until 24 hours after the VM is created or CPU, RAM, or CPU class is increased.",
				Validators: []validator.String{
					stringvalidator.OneOf("running", "stopped"),
				},
			},
			"wan_mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("dual"),
				MarkdownDescription: "Public network: `off`, `ipv4`, `ipv6` or `dual` (default). With `off`, attach at least one private network. Switching between `ipv4`, `ipv6` and `dual` requires going through `off`.",
				Validators: []validator.String{
					stringvalidator.OneOf("off", "ipv4", "ipv6", "dual"),
				},
			},
			"wan_ip": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Reserved project IPv4 to attach. Omit to take the next free address. Changing it replaces the VM.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(
						func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
							resp.RequiresReplace = !req.StateValue.IsNull()
						},
						"Changing the public IPv4 replaces the VM.",
						"Changing the public IPv4 replaces the VM.",
					),
				},
			},
			"wan_ipv6": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Reserved project IPv6 to attach. Omit to take the next free address. Changing it replaces the VM.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(
						func(_ context.Context, req planmodifier.StringRequest, resp *stringplanmodifier.RequiresReplaceIfFuncResponse) {
							resp.RequiresReplace = !req.StateValue.IsNull()
						},
						"Changing the public IPv6 replaces the VM.",
						"Changing the public IPv6 replaces the VM.",
					),
				},
			},
			"retain_public_ips": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				MarkdownDescription: "When destroying the VM, or turning public networking off, keep its public addresses reserved in the project. A reserved IPv4 stays billed. Defaults to false, which releases the addresses.",
			},
			"lan_interfaces": schema.ListNestedAttribute{
				Optional: true,
				MarkdownDescription: "Private networks to attach. `mode` `auto` takes a free address; `select` uses `ip`. " +
					"When `wan_mode` is `off` and more than one network is attached, set `is_gateway` on exactly one. Create the networks in the portal first.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"vnet": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Private network name (VNet).",
						},
						"mode": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "`auto` or `select`.",
							Validators: []validator.String{
								stringvalidator.OneOf("auto", "select"),
							},
						},
						"ip": schema.StringAttribute{
							Optional:            true,
							MarkdownDescription: "IPv4 address. Required when `mode` is `select`.",
						},
						"is_gateway": schema.BoolAttribute{
							Optional:            true,
							Computed:            true,
							Default:             booldefault.StaticBool(false),
							MarkdownDescription: "Use this network as the default gateway. Required on exactly one interface when WAN is off and several networks are attached.",
						},
					},
				},
			},
			"ipv4": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public IPv4 address, empty when none is attached. Stays known in the plan while `wan_mode` does not change.",
			},
			"ipv6": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Public IPv6 address, empty when none is attached. Stays known in the plan while `wan_mode` does not change.",
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create:            true,
				Update:            true,
				Delete:            true,
				CreateDescription: "How long to wait for creation and for `power_state`. Defaults to 10 minutes.",
				UpdateDescription: "How long to wait for a resize, shutdown, or start. Defaults to 15 minutes.",
				DeleteDescription: "How long to wait for the VM to stop and be deleted. Defaults to 10 minutes.",
			}),
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
		WANIP:    data.WANIP.ValueString(),
		WANIPv6:  data.WANIPv6.ValueString(),
	}
	resp.Diagnostics.Append(data.SSHKeyIDs.ElementsAs(ctx, &in.SSHKeyIDs, false)...)
	var lanDiags diag.Diagnostics
	in.LANInterfaces, lanDiags = data.lanClients(ctx)
	resp.Diagnostics.Append(lanDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if msg := data.lanError(ctx); msg != "" {
		resp.Diagnostics.AddError("Invalid private networks", msg)
		return
	}
	ctx, cancel, ok := operationContext(ctx, data.Timeouts.Create, vmCreateTimeout, &resp.Diagnostics)
	if !ok {
		return
	}
	defer cancel()

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
	if data.PowerState.ValueString() == "stopped" {
		if err := r.client.SetPower(ctx, projectID, vmid, "shutdown"); err != nil {
			resp.Diagnostics.AddError("Error stopping VM", fmt.Sprintf("stopping VM %d: %s", vmid, err))
			return
		}
		if err := r.client.WaitVMStatus(ctx, projectID, vmid, "stopped"); err != nil {
			resp.Diagnostics.AddError("Error stopping VM", fmt.Sprintf("waiting for VM %d to stop: %s", vmid, err))
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

func (r *vmResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var plan vmResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if msg := plan.lanError(ctx); msg != "" {
		resp.Diagnostics.AddError("Invalid private networks", msg)
	}
	if req.State.Raw.IsNull() {
		planAddresses(&plan, nil)
		resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
		return
	}
	var state vmResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	planAddresses(&plan, &state)
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
	if msg := wanModeChangeError(state.WANMode, plan.WANMode); msg != "" {
		resp.Diagnostics.AddError("Cannot switch public network", msg)
	}
	if downgradeBlocked(plan, state) {
		resp.Diagnostics.AddError(
			"Cannot lower CPU or RAM on a running VM",
			`Set power_state to "stopped". The API also refuses a downgrade until 24 hours after the VM is created or CPU, RAM, or CPU class is increased.`,
		)
	}
}

func (r *vmResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data vmResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if attribute, msg := vmSizeError(data.Cores, data.RAMGB, data.OSDiskGB); msg != "" {
		resp.Diagnostics.AddAttributeError(path.Root(attribute), "Invalid VM size", msg)
	}
}

// vmSizeError mirrors the API limits on vCPU, RAM and OS disk size.
func vmSizeError(cores, ramGB, diskGB types.Int64) (string, string) {
	if cores.IsNull() || cores.IsUnknown() {
		return "", ""
	}
	c := cores.ValueInt64()
	if c < 2 || c%2 != 0 {
		return "cores", fmt.Sprintf("cores must be an even number, at least 2; got %d.", c)
	}
	if !ramGB.IsNull() && !ramGB.IsUnknown() {
		if ram := ramGB.ValueInt64(); ram < c || ram > 4*c {
			return "ram_gb", fmt.Sprintf("With %d vCPU, ram_gb must be between %d and %d (1 to 4 GiB per vCPU); got %d.", c, c, 4*c, ram)
		}
	}
	if !diskGB.IsNull() && !diskGB.IsUnknown() {
		if disk := diskGB.ValueInt64(); disk%20 != 0 || disk < 10*c || disk > 100*c {
			return "os_disk_gb", fmt.Sprintf("With %d vCPU, os_disk_gb must be a multiple of 20 between %d and %d (10 to 100 GiB per vCPU); got %d.", c, 10*c, 100*c, disk)
		}
	}
	return "", ""
}

func (r *vmResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vmResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if downgradeBlocked(plan, state) {
		resp.Diagnostics.AddError(
			"Cannot lower CPU or RAM on a running VM",
			`Set power_state to "stopped". The API also refuses a downgrade until 24 hours after the VM is created or CPU, RAM, or CPU class is increased.`,
		)
		return
	}
	if msg := plan.lanError(ctx); msg != "" {
		resp.Diagnostics.AddError("Invalid private networks", msg)
		return
	}
	if msg := wanModeChangeError(state.WANMode, plan.WANMode); msg != "" {
		resp.Diagnostics.AddError("Cannot switch public network", msg)
		return
	}
	ctx, cancel, ok := operationContext(ctx, plan.Timeouts.Update, vmUpdateTimeout, &resp.Diagnostics)
	if !ok {
		return
	}
	defer cancel()

	planInput, diags := plan.upgradeInput(ctx)
	resp.Diagnostics.Append(diags...)
	stateInput, diags := state.upgradeInput(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectID, vmid := plan.ProjectID.ValueString(), plan.VMID.ValueInt64()
	want, current := plan.PowerState.ValueString(), state.PowerState.ValueString()
	if want == "stopped" && current != "stopped" {
		// A paused guest cannot shut down cleanly.
		action := "shutdown"
		if current != "running" {
			action = "stop"
		}
		if err := r.client.SetPower(ctx, projectID, vmid, action); err != nil {
			resp.Diagnostics.AddError("Error stopping VM", fmt.Sprintf("stopping VM %d: %s", vmid, err))
			return
		}
		if err := r.client.WaitVMStatus(ctx, projectID, vmid, "stopped"); err != nil {
			resp.Diagnostics.AddError("Error stopping VM", fmt.Sprintf("waiting for VM %d to stop: %s", vmid, err))
			return
		}
	}
	if !reflect.DeepEqual(planInput, stateInput) {
		planInput.ReleasePublicIP = !plan.RetainPublicIPs.ValueBool()
		if err := r.client.UpgradeVM(ctx, projectID, vmid, planInput); err != nil {
			resp.Diagnostics.AddError(
				"Error updating VM",
				fmt.Sprintf("updating VM %d: %s", vmid, err),
			)
			return
		}
	}
	if want == "running" && current != "running" {
		action := "start"
		if current != "stopped" {
			action = "resume"
		}
		if err := r.client.SetPower(ctx, projectID, vmid, action); err != nil {
			resp.Diagnostics.AddError("Error starting VM", fmt.Sprintf("starting VM %d: %s", vmid, err))
			return
		}
		if err := r.client.WaitVMStatus(ctx, projectID, vmid, "running"); err != nil {
			resp.Diagnostics.AddError("Error starting VM", fmt.Sprintf("waiting for VM %d to run: %s", vmid, err))
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
	ctx, cancel, ok := operationContext(ctx, data.Timeouts.Delete, vmDeleteTimeout, &resp.Diagnostics)
	if !ok {
		return
	}
	defer cancel()

	err := r.client.DestroyVM(ctx, data.ProjectID.ValueString(), data.VMID.ValueInt64(), !data.RetainPublicIPs.ValueBool())
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

func operationContext(ctx context.Context, timeout func(context.Context, time.Duration) (time.Duration, diag.Diagnostics), fallback time.Duration, diags *diag.Diagnostics) (context.Context, context.CancelFunc, bool) {
	d, more := timeout(ctx, fallback)
	diags.Append(more...)
	if diags.HasError() {
		return ctx, func() {}, false
	}
	ctx, cancel := context.WithTimeout(ctx, d)
	return ctx, cancel, true
}

func downgradeBlocked(plan, state vmResourceModel) bool {
	if plan.PowerState.ValueString() != "running" {
		return false
	}
	if plan.Cores.IsUnknown() || state.Cores.IsUnknown() || plan.RAMGB.IsUnknown() || state.RAMGB.IsUnknown() {
		return false
	}
	return plan.Cores.ValueInt64() < state.Cores.ValueInt64() || plan.RAMGB.ValueInt64() < state.RAMGB.ValueInt64()
}

// upgradeInput is the resizable part of the model in the shape the API expects.
func (m vmResourceModel) upgradeInput(ctx context.Context) (client.UpgradeVMInput, diag.Diagnostics) {
	lan, diags := m.lanClients(ctx)
	if lan == nil {
		lan = []client.LANInterface{}
	}
	return client.UpgradeVMInput{
		Cores:         m.Cores.ValueInt64(),
		RAMGB:         m.RAMGB.ValueInt64(),
		OSDiskGB:      m.OSDiskGB.ValueInt64(),
		CPUMode:       m.CPUMode.ValueString(),
		WANMode:       m.WANMode.ValueString(),
		WANIP:         m.WANIP.ValueString(),
		WANIPv6:       m.WANIPv6.ValueString(),
		LANInterfaces: lan,
	}, diags
}

func (m vmResourceModel) lanClients(ctx context.Context) ([]client.LANInterface, diag.Diagnostics) {
	lans, diags := m.lanModels(ctx)
	if diags.HasError() {
		return nil, diags
	}
	out := make([]client.LANInterface, 0, len(lans))
	for _, lan := range lans {
		out = append(out, client.LANInterface{
			VNet:      lan.VNet.ValueString(),
			Mode:      lan.Mode.ValueString(),
			IP:        lan.IP.ValueString(),
			IsGateway: lan.IsGateway.ValueBool(),
		})
	}
	return out, diags
}

func (m vmResourceModel) lanModels(ctx context.Context) ([]lanInterfaceModel, diag.Diagnostics) {
	if m.LANInterfaces.IsNull() || m.LANInterfaces.IsUnknown() {
		return nil, nil
	}
	var lans []lanInterfaceModel
	diags := m.LANInterfaces.ElementsAs(ctx, &lans, false)
	return lans, diags
}

func (m vmResourceModel) lanError(ctx context.Context) string {
	lans, diags := m.lanModels(ctx)
	if diags.HasError() {
		return ""
	}
	wanOff := !m.WANMode.IsUnknown() && m.WANMode.ValueString() == "off"
	if wanOff && len(lans) == 0 && !m.LANInterfaces.IsUnknown() {
		return "With wan_mode off, attach at least one private network in lan_interfaces."
	}
	gateways := 0
	for _, lan := range lans {
		if lan.Mode.IsUnknown() || lan.IP.IsUnknown() || lan.IsGateway.IsUnknown() {
			return ""
		}
		if lan.Mode.ValueString() == "select" && lan.IP.ValueString() == "" {
			return "ip is required when mode is select."
		}
		if lan.IsGateway.ValueBool() {
			gateways++
		}
	}
	if wanOff && len(lans) > 1 && gateways != 1 {
		return "When wan_mode is off and more than one private network is attached, set is_gateway on exactly one."
	}
	return ""
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
	// Kept as reported (e.g. paused) so the plan shows the change back to power_state.
	m.PowerState = types.StringValue(vm.Status)
	m.WANMode = types.StringValue(wanMode(cfg.WAN))
	if m.RetainPublicIPs.IsNull() {
		// Not stored by the API; an import starts from the default.
		m.RetainPublicIPs = types.BoolValue(false)
	}
	m.setAddresses(cfg)
	return m.setLAN(ctx, cfg.LANInterfaces)
}

func (m *vmResourceModel) setLAN(ctx context.Context, api []client.VMLAN) diag.Diagnostics {
	prev, diags := m.lanModels(ctx)
	if diags.HasError() {
		return diags
	}
	byVNet := make(map[string]client.VMLAN, len(api))
	for _, lan := range api {
		byVNet[lan.VNet] = lan
	}
	onlyOne := len(api) == 1
	ordered := make([]lanInterfaceModel, 0, len(api))
	seen := make(map[string]bool, len(api))
	for _, item := range prev {
		lan, ok := byVNet[item.VNet.ValueString()]
		if !ok {
			continue
		}
		ordered = append(ordered, mergeLAN(item, lan, onlyOne))
		seen[lan.VNet] = true
	}
	var extra []string
	for vnet := range byVNet {
		if !seen[vnet] {
			extra = append(extra, vnet)
		}
	}
	sort.Strings(extra)
	blank := lanInterfaceModel{Mode: types.StringValue("auto"), IsGateway: types.BoolValue(false)}
	for _, vnet := range extra {
		ordered = append(ordered, mergeLAN(blank, byVNet[vnet], onlyOne))
	}
	if len(ordered) == 0 {
		m.LANInterfaces = types.ListNull(types.ObjectType{AttrTypes: lanInterfaceAttrTypes})
		return nil
	}
	m.LANInterfaces, diags = types.ListValueFrom(ctx, types.ObjectType{AttrTypes: lanInterfaceAttrTypes}, ordered)
	return diags
}

func mergeLAN(prev lanInterfaceModel, api client.VMLAN, onlyOne bool) lanInterfaceModel {
	mode := prev.Mode.ValueString()
	if mode == "" {
		mode = "auto"
	}
	ip := types.StringNull()
	if mode == "select" {
		if api.IP != nil {
			ip = types.StringValue(*api.IP)
		} else if !prev.IP.IsNull() && !prev.IP.IsUnknown() {
			ip = prev.IP
		}
	}
	gateway := types.BoolValue(api.IsGateway)
	if onlyOne && !prev.IsGateway.IsNull() && !prev.IsGateway.IsUnknown() {
		gateway = prev.IsGateway
	}
	return lanInterfaceModel{
		VNet:      types.StringValue(api.VNet),
		Mode:      types.StringValue(mode),
		IP:        ip,
		IsGateway: gateway,
	}
}

func wanModeChangeError(from, to types.String) string {
	if from.IsNull() || from.IsUnknown() || to.IsNull() || to.IsUnknown() || from.Equal(to) {
		return ""
	}
	attached := from.ValueString() != "off" && to.ValueString() != "off"
	if attached {
		return `Set wan_mode to "off" before changing between ipv4, ipv6 and dual.`
	}
	return ""
}

// planAddresses keeps ipv4 and ipv6 in the plan while wan_mode stays the same.
// A mode that does not include an address makes that address null.
func planAddresses(plan, state *vmResourceModel) {
	if plan.WANMode.IsUnknown() || plan.WANMode.IsNull() {
		return
	}
	same := state != nil && !state.WANMode.IsUnknown() && state.WANMode.Equal(plan.WANMode)
	switch plan.WANMode.ValueString() {
	case "off":
		plan.IPv4 = types.StringNull()
		plan.IPv6 = types.StringNull()
	case "ipv4":
		plan.IPv6 = types.StringNull()
		if same {
			plan.IPv4 = state.IPv4
		}
	case "ipv6":
		plan.IPv4 = types.StringNull()
		if same {
			plan.IPv6 = state.IPv6
		}
	case "dual":
		if same {
			plan.IPv4 = state.IPv4
			plan.IPv6 = state.IPv6
		}
	}
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
