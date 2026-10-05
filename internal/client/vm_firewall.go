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

type VMFirewallRuleInput struct {
	Position *int64 `json:"position,omitempty"`
	Pos      *int64 `json:"pos,omitempty"`
	Type     string `json:"type"`
	Action   string `json:"action"`
	Proto    string `json:"proto,omitempty"`
	Source   string `json:"source"`
	Dest     string `json:"dest"`
	SPort    string `json:"sport"`
	DPort    string `json:"dport"`
	Comment  string `json:"comment"`
	Enable   bool   `json:"enable"`
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

// VMFirewallRule is a packet filter or an attached security group. Group rows leave proto empty.
type VMFirewallRule struct {
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

func (c *Client) ListVMFirewallRules(ctx context.Context, projectUUID string, vmid int64) ([]VMFirewallRule, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, vmFirewallPath(projectUUID, vmid)+"rules/", nil, &raw); err != nil {
		return nil, err
	}
	if string(raw) == "null" {
		return nil, fmt.Errorf("VM firewall rules are unavailable")
	}
	var rules []VMFirewallRule
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func (c *Client) GetVMFirewallRule(ctx context.Context, projectUUID string, vmid, pos int64) (*VMFirewallRule, error) {
	rules, err := c.ListVMFirewallRules(ctx, projectUUID, vmid)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].Pos == pos {
			return &rules[i], nil
		}
	}
	return nil, &Error{Status: http.StatusNotFound, Body: fmt.Sprintf("VM firewall rule at position %d not found", pos)}
}

func (c *Client) CreateVMFirewallRule(ctx context.Context, projectUUID string, vmid int64, rule VMFirewallRuleInput) error {
	return c.do(ctx, http.MethodPost, vmFirewallPath(projectUUID, vmid)+"rules/", rule, nil)
}

func (c *Client) UpdateVMFirewallRule(ctx context.Context, projectUUID string, vmid int64, rule VMFirewallRuleInput) error {
	return c.do(ctx, http.MethodPut, vmFirewallPath(projectUUID, vmid)+"rules/", rule, nil)
}

func (c *Client) DeleteVMFirewallRule(ctx context.Context, projectUUID string, vmid, pos int64) error {
	return c.do(ctx, http.MethodDelete, vmFirewallPath(projectUUID, vmid)+"rules/", map[string]int64{"pos": pos}, nil)
}

func (c *Client) MoveVMFirewallRule(ctx context.Context, projectUUID string, vmid, from, to int64) error {
	in := map[string]int64{"from_pos": from, "to_pos": to}
	return c.do(ctx, http.MethodPost, vmFirewallPath(projectUUID, vmid)+"rules/move/", in, nil)
}
