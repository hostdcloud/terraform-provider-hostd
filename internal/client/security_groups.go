// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type SecurityGroup struct {
	UUID         string `json:"uuid"`
	DisplayName  string `json:"display_name"`
	PVEGroupName string `json:"pve_group_name"`
	Comment      string `json:"comment"`
}

type SecurityGroupRule struct {
	Pos     int64   `json:"pos"`
	Type    string  `json:"type"`
	Action  string  `json:"action"`
	Proto   string  `json:"proto"`
	Source  *string `json:"source"`
	Dest    *string `json:"dest"`
	SPort   *string `json:"sport"`
	DPort   *string `json:"dport"`
	Comment *string `json:"comment"`
	Enable  *bool   `json:"enable"`
}

// SecurityGroupRuleInput is a create or update body. Set Position to insert,
// Pos to update the rule that already sits there.
type SecurityGroupRuleInput struct {
	Position *int64 `json:"position,omitempty"`
	Pos      *int64 `json:"pos,omitempty"`
	Type     string `json:"type"`
	Action   string `json:"action"`
	Proto    string `json:"proto"`
	Source   string `json:"source"`
	Dest     string `json:"dest"`
	SPort    string `json:"sport"`
	DPort    string `json:"dport"`
	Comment  string `json:"comment"`
	Enable   bool   `json:"enable"`
}

func groupPath(projectUUID, uuid string) string {
	return "/cloud/project/" + projectUUID + "/firewall/groups/" + uuid + "/"
}

func (c *Client) GetSecurityGroup(ctx context.Context, projectUUID, uuid string) (*SecurityGroup, error) {
	var groups []SecurityGroup
	if err := c.do(ctx, http.MethodGet, "/cloud/project/"+projectUUID+"/firewall/groups/", nil, &groups); err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].UUID == uuid {
			return &groups[i], nil
		}
	}
	return nil, &Error{Status: http.StatusNotFound, Body: fmt.Sprintf("security group %s not found", uuid)}
}

func (c *Client) CreateSecurityGroup(ctx context.Context, projectUUID, name, comment string) (*SecurityGroup, error) {
	var group SecurityGroup
	in := map[string]string{"display_name": name, "comment": comment}
	if err := c.do(ctx, http.MethodPost, "/cloud/project/"+projectUUID+"/firewall/groups/", in, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

func (c *Client) UpdateSecurityGroup(ctx context.Context, projectUUID, uuid, name, comment string) (*SecurityGroup, error) {
	var group SecurityGroup
	in := map[string]string{"display_name": name, "comment": comment}
	if err := c.do(ctx, http.MethodPatch, groupPath(projectUUID, uuid), in, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

func (c *Client) DeleteSecurityGroup(ctx context.Context, projectUUID, uuid string) error {
	return c.do(ctx, http.MethodDelete, groupPath(projectUUID, uuid), nil, nil)
}

func (c *Client) ListSecurityGroupRules(ctx context.Context, projectUUID, uuid string) ([]SecurityGroupRule, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, groupPath(projectUUID, uuid)+"rules/", nil, &raw); err != nil {
		return nil, err
	}
	if string(raw) == "null" {
		return nil, fmt.Errorf("security group rules are unavailable")
	}
	var rules []SecurityGroupRule
	if err := json.Unmarshal(raw, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func (c *Client) GetSecurityGroupRule(ctx context.Context, projectUUID, uuid string, pos int64) (*SecurityGroupRule, error) {
	rules, err := c.ListSecurityGroupRules(ctx, projectUUID, uuid)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		if rules[i].Pos == pos {
			return &rules[i], nil
		}
	}
	return nil, &Error{Status: http.StatusNotFound, Body: fmt.Sprintf("security group rule at position %d not found", pos)}
}

func (c *Client) CreateSecurityGroupRule(ctx context.Context, projectUUID, uuid string, rule SecurityGroupRuleInput) error {
	return c.do(ctx, http.MethodPost, groupPath(projectUUID, uuid)+"rules/", rule, nil)
}

func (c *Client) UpdateSecurityGroupRule(ctx context.Context, projectUUID, uuid string, rule SecurityGroupRuleInput) error {
	return c.do(ctx, http.MethodPut, groupPath(projectUUID, uuid)+"rules/", rule, nil)
}

func (c *Client) DeleteSecurityGroupRule(ctx context.Context, projectUUID, uuid string, pos int64) error {
	return c.do(ctx, http.MethodDelete, groupPath(projectUUID, uuid)+"rules/", map[string]int64{"pos": pos}, nil)
}

func (c *Client) MoveSecurityGroupRule(ctx context.Context, projectUUID, uuid string, from, to int64) error {
	in := map[string]int64{"from_pos": from, "to_pos": to}
	return c.do(ctx, http.MethodPost, groupPath(projectUUID, uuid)+"rules/move/", in, nil)
}
