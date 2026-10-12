// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
)

type SecurityGroup struct {
	UUID         string `json:"uuid"`
	DisplayName  string `json:"display_name"`
	PVEGroupName string `json:"pve_group_name"`
	Comment      string `json:"comment"`
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

func (c *Client) ListSecurityGroupRules(ctx context.Context, projectUUID, uuid string) ([]FirewallRule, error) {
	return c.listRules(ctx, groupPath(projectUUID, uuid)+"rules/")
}

// ReplaceSecurityGroupRules sets the full ordered rule list of the group.
func (c *Client) ReplaceSecurityGroupRules(ctx context.Context, projectUUID, uuid string, rules []FirewallRuleInput) error {
	return c.do(ctx, http.MethodPut, groupPath(projectUUID, uuid)+"rules/replace/", map[string]any{"rules": rules}, nil)
}
