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
	VNet string `json:"vnet"`
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
	Hostname  string   `json:"hostname"`
	Username  string   `json:"username,omitempty"`
	Password  string   `json:"password"`
	SSHKeyIDs []int64  `json:"ssh_key_ids,omitempty"`
	Cores     int64    `json:"cores"`
	RAMGB     int64    `json:"ram_gb"`
	OSDiskGB  int64    `json:"os_disk_gb"`
	OSImage   string   `json:"os_image"`
	CPUMode   string   `json:"cpu_mode"`
	WANMode   string   `json:"wan_mode"`
	LANVNets  []string `json:"lan_vnets,omitempty"`
}

type LANInterface struct {
	VNet string `json:"vnet"`
	Mode string `json:"mode"`
}

// UpgradeVMInput is the full desired size and networking: the API releases
// every private network missing from LANInterfaces.
type UpgradeVMInput struct {
	Cores         int64          `json:"cores"`
	RAMGB         int64          `json:"ram_gb"`
	OSDiskGB      int64          `json:"os_disk_gb"`
	CPUMode       string         `json:"cpu_mode"`
	WANMode       string         `json:"wan_mode"`
	LANInterfaces []LANInterface `json:"lan_interfaces"`
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
func (c *Client) CreateVM(ctx context.Context, projectUUID string, in CreateVMInput) (int64, error) {
	var out struct {
		VMID int64 `json:"vmid"`
	}
	if err := c.do(ctx, http.MethodPost, "/cloud/project/"+projectUUID+"/vm/", in, &out); err != nil {
		return 0, err
	}
	return out.VMID, nil
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

// WaitVMStatus polls until the VM reports the wanted power status.
func (c *Client) WaitVMStatus(ctx context.Context, projectUUID string, vmid int64, want string) error {
	ctx, cancel := context.WithTimeout(ctx, vmWaitTimeout)
	defer cancel()

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
// disks and backups. Public IPs are released.
func (c *Client) DestroyVM(ctx context.Context, projectUUID string, vmid int64) error {
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
	return c.do(ctx, http.MethodDelete, vmPath(projectUUID, vmid), nil, nil)
}
