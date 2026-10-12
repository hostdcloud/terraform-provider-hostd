// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type VMFirewallOptions struct {
	Enable    *bool   `json:"enable"`
	PolicyIn  *string `json:"policy_in"`
	PolicyOut *string `json:"policy_out"`
}

// FirewallRuleInput is one rule of a full rule list. Proto is empty for a
// security group attached to a VM.
type FirewallRuleInput struct {
	Type    string `json:"type"`
	Action  string `json:"action"`
	Proto   string `json:"proto,omitempty"`
	Source  string `json:"source"`
	Dest    string `json:"dest"`
	SPort   string `json:"sport"`
	DPort   string `json:"dport"`
	Comment string `json:"comment"`
	Enable  bool   `json:"enable"`
}

func vmFirewallPath(projectUUID string, vmid int64) string {
	return fmt.Sprintf("/cloud/project/%s/vm/%d/firewall/", projectUUID, vmid)
}

func (c *Client) GetVMFirewallOptions(ctx context.Context, projectUUID string, vmid int64) (*VMFirewallOptions, error) {
	var options VMFirewallOptions
	if err := c.do(ctx, http.MethodGet, vmFirewallPath(projectUUID, vmid)+"config/", nil, &options); err != nil {
		return nil, err
	}
	return &options, nil
}

func (c *Client) SetVMFirewallOptions(ctx context.Context, projectUUID string, vmid int64, enable bool, policyIn, policyOut string) error {
	in := map[string]any{"enable": enable, "policy_in": policyIn, "policy_out": policyOut}
	return c.do(ctx, http.MethodPut, vmFirewallPath(projectUUID, vmid)+"config/", in, nil)
}

// FirewallRule is a packet filter or, on a VM, an attached security group.
type FirewallRule struct {
	Pos     int64   `json:"pos"`
	Type    string  `json:"type"`
	Action  *string `json:"action"`
	Proto   *string `json:"proto"`
	Source  *string `json:"source"`
	Dest    *string `json:"dest"`
	SPort   *string `json:"sport"`
	DPort   *string `json:"dport"`
	Comment *string `json:"comment"`
	Enable  *bool   `json:"enable"`
}

// listRules reads a rule list; null means the API could not read it.
func (c *Client) listRules(ctx context.Context, path string) ([]FirewallRule, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return nil, err
	}
	if string(raw) == "null" {
		return nil, fmt.Errorf("firewall rules are unavailable, retry later")
	}
	var rules []FirewallRule
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func (c *Client) ListVMFirewallRules(ctx context.Context, projectUUID string, vmid int64) ([]FirewallRule, error) {
	return c.listRules(ctx, vmFirewallPath(projectUUID, vmid)+"rules/")
}

// ReplaceVMFirewallRules sets the full ordered rule list of the VM.
func (c *Client) ReplaceVMFirewallRules(ctx context.Context, projectUUID string, vmid int64, rules []FirewallRuleInput) error {
	return c.do(ctx, http.MethodPut, vmFirewallPath(projectUUID, vmid)+"rules/replace/", map[string]any{"rules": rules}, nil)
}
