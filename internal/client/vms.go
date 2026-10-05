// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

const vmWaitTimeout = 5 * time.Minute

// vmPollInterval is a variable so tests can shorten it.
var vmPollInterval = 5 * time.Second

type VM struct {
	Status  string `json:"status"`
	CPUMode string `json:"cpu_mode"`
}

type VMWAN struct {
	IPv4 *string `json:"ipv4"`
	IPv6 *string `json:"ipv6"`
}

type VMLAN struct {
	VNet      string  `json:"vnet"`
	IP        *string `json:"ip"`
	IsGateway bool    `json:"is_gateway"`
}

type VMConfig struct {
	Name          string  `json:"name"`
	Cores         int64   `json:"cores"`
	RAMGB         int64   `json:"ram_gb"`
	OSDiskGB      *int64  `json:"os_disk_gb"`
	WAN           *VMWAN  `json:"wan"`
	LANInterfaces []VMLAN `json:"lan_interfaces"`
}

type CreateVMInput struct {
	Hostname      string         `json:"hostname"`
	Username      string         `json:"username,omitempty"`
	Password      string         `json:"password"`
	SSHKeyIDs     []int64        `json:"ssh_key_ids,omitempty"`
	Cores         int64          `json:"cores"`
	RAMGB         int64          `json:"ram_gb"`
	OSDiskGB      int64          `json:"os_disk_gb"`
	OSImage       string         `json:"os_image"`
	CPUMode       string         `json:"cpu_mode"`
	WANMode       string         `json:"wan_mode"`
	WANIP         string         `json:"wan_ip,omitempty"`
	WANIPv6       string         `json:"wan_ipv6,omitempty"`
	LANInterfaces []LANInterface `json:"lan_interfaces,omitempty"`
}

type LANInterface struct {
	VNet      string `json:"vnet"`
	Mode      string `json:"mode"`
	IP        string `json:"ip,omitempty"`
	IsGateway bool   `json:"is_gateway"`
}

// UpgradeVMInput is the full desired size and networking: the API releases
// every private network missing from LANInterfaces.
type UpgradeVMInput struct {
	Cores           int64          `json:"cores"`
	RAMGB           int64          `json:"ram_gb"`
	OSDiskGB        int64          `json:"os_disk_gb"`
	CPUMode         string         `json:"cpu_mode"`
	WANMode         string         `json:"wan_mode"`
	WANIP           string         `json:"wan_ip,omitempty"`
	WANIPv6         string         `json:"wan_ipv6,omitempty"`
	ReleasePublicIP bool           `json:"release_public_ip"`
	LANInterfaces   []LANInterface `json:"lan_interfaces"`
}

// ListOSImages returns the image names available for new VMs in the project.
func (c *Client) ListOSImages(ctx context.Context, projectUUID string) ([]string, error) {
	var images []struct {
		Name string `json:"name"`
	}
	if err := c.do(ctx, http.MethodGet, "/cloud/project/"+projectUUID+"/cluster/available-images/", nil, &images); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(images))
	for _, image := range images {
		names = append(names, image.Name)
	}
	return names, nil
}

func vmPath(projectUUID string, vmid int64) string {
	return fmt.Sprintf("/cloud/project/%s/vm/%d/", projectUUID, vmid)
}

// CreateVM creates and starts a VM; the API returns once creation has finished.
// If the call is interrupted, a VM with the same hostname in the project is
// adopted so a retry does not create a second one.
func (c *Client) CreateVM(ctx context.Context, projectUUID string, in CreateVMInput) (int64, error) {
	var out struct {
		VMID int64 `json:"vmid"`
	}
	err := c.do(ctx, http.MethodPost, "/cloud/project/"+projectUUID+"/vm/", in, &out)
	if err == nil {
		return out.VMID, nil
	}
	if !isInterrupted(err) {
		return 0, err
	}
	vmid, found, findErr := c.vmByHostname(context.WithoutCancel(ctx), projectUUID, in.Hostname)
	if findErr != nil {
		return 0, findErr
	}
	if found {
		return vmid, nil
	}
	return 0, err
}

func (c *Client) vmByHostname(ctx context.Context, projectUUID, hostname string) (int64, bool, error) {
	var vms []struct {
		VMID int64  `json:"vmid"`
		Name string `json:"name"`
	}
	if err := c.do(ctx, http.MethodGet, "/cloud/project/"+projectUUID+"/vm/", nil, &vms); err != nil {
		return 0, false, nil
	}
	var vmid int64
	matches := 0
	for _, vm := range vms {
		if vm.Name == hostname {
			matches++
			vmid = vm.VMID
		}
	}
	if matches > 1 {
		return 0, false, fmt.Errorf("%d VMs in the project are named %q; import the one you want as <project_id>/<vmid>", matches, hostname)
	}
	return vmid, matches == 1, nil
}

func (c *Client) GetVM(ctx context.Context, projectUUID string, vmid int64) (*VM, error) {
	var vm VM
	if err := c.do(ctx, http.MethodGet, vmPath(projectUUID, vmid), nil, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

func (c *Client) GetVMConfig(ctx context.Context, projectUUID string, vmid int64) (*VMConfig, error) {
	var cfg VMConfig
	if err := c.do(ctx, http.MethodGet, vmPath(projectUUID, vmid)+"config/current/", nil, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Client) UpgradeVM(ctx context.Context, projectUUID string, vmid int64, in UpgradeVMInput) error {
	return c.do(ctx, http.MethodPost, vmPath(projectUUID, vmid)+"upgrade/", in, nil)
}

// SetPower runs a power action: start, shutdown, or stop.
func (c *Client) SetPower(ctx context.Context, projectUUID string, vmid int64, action string) error {
	return c.do(ctx, http.MethodPost, vmPath(projectUUID, vmid)+"power/"+action+"/", nil, nil)
}

// WaitVMStatus polls until the VM reports the wanted power status.
// A deadline already on ctx is kept; otherwise the wait is vmWaitTimeout.
func (c *Client) WaitVMStatus(ctx context.Context, projectUUID string, vmid int64, want string) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, vmWaitTimeout)
		defer cancel()
	}

	for {
		vm, err := c.GetVM(ctx, projectUUID, vmid)
		if err != nil {
			return err
		}
		if vm.Status == want {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("VM %d is %q, expected %q: %w", vmid, vm.Status, want, ctx.Err())
		case <-time.After(vmPollInterval):
		}
	}
}

// DestroyVM hard-stops the VM when it is not stopped, then deletes it with its
// disks and backups. releasePublicIP returns public addresses to the pool;
// false keeps them reserved on the project.
func (c *Client) DestroyVM(ctx context.Context, projectUUID string, vmid int64, releasePublicIP bool) error {
	vm, err := c.GetVM(ctx, projectUUID, vmid)
	if err != nil {
		return err
	}
	if vm.Status != "stopped" {
		if err := c.do(ctx, http.MethodPost, vmPath(projectUUID, vmid)+"power/stop/", nil, nil); err != nil {
			return err
		}
		if err := c.WaitVMStatus(ctx, projectUUID, vmid, "stopped"); err != nil {
			return err
		}
	}
	release := "true"
	if !releasePublicIP {
		release = "false"
	}
	return c.do(ctx, http.MethodDelete, vmPath(projectUUID, vmid)+"?release_public_ip="+release, nil, nil)
}
