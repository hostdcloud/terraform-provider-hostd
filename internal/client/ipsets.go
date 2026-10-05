// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
)

type IPSet struct {
	UUID         string `json:"uuid"`
	DisplayName  string `json:"display_name"`
	PVEIPSetName string `json:"pve_ipset_name"`
	Comment      string `json:"comment"`
}

type IPSetMember struct {
	CIDR    string `json:"cidr"`
	Comment string `json:"comment"`
	NoMatch bool   `json:"nomatch"`
}

func ipSetPath(projectUUID, uuid string) string {
	return "/cloud/project/" + projectUUID + "/firewall/ipsets/" + uuid + "/"
}

func (c *Client) ListIPSets(ctx context.Context, projectUUID string) ([]IPSet, error) {
	var sets []IPSet
	if err := c.do(ctx, http.MethodGet, "/cloud/project/"+projectUUID+"/firewall/ipsets/", nil, &sets); err != nil {
		return nil, err
	}
	return sets, nil
}

func (c *Client) GetIPSet(ctx context.Context, projectUUID, uuid string) (*IPSet, error) {
	sets, err := c.ListIPSets(ctx, projectUUID)
	if err != nil {
		return nil, err
	}
	for i := range sets {
		if sets[i].UUID == uuid {
			return &sets[i], nil
		}
	}
	return nil, &Error{Status: http.StatusNotFound, Body: fmt.Sprintf("IP set %s not found", uuid)}
}

func (c *Client) CreateIPSet(ctx context.Context, projectUUID, name, comment string) (*IPSet, error) {
	var set IPSet
	in := map[string]string{"display_name": name, "comment": comment}
	if err := c.do(ctx, http.MethodPost, "/cloud/project/"+projectUUID+"/firewall/ipsets/", in, &set); err != nil {
		return nil, err
	}
	return &set, nil
}

func (c *Client) UpdateIPSet(ctx context.Context, projectUUID, uuid, name, comment string) (*IPSet, error) {
	var set IPSet
	in := map[string]string{"display_name": name, "comment": comment}
	if err := c.do(ctx, http.MethodPatch, ipSetPath(projectUUID, uuid), in, &set); err != nil {
		return nil, err
	}
	return &set, nil
}

func (c *Client) DeleteIPSet(ctx context.Context, projectUUID, uuid string) error {
	return c.do(ctx, http.MethodDelete, ipSetPath(projectUUID, uuid), nil, nil)
}

func (c *Client) ListIPSetMembers(ctx context.Context, projectUUID, uuid string) ([]IPSetMember, error) {
	var members []IPSetMember
	if err := c.do(ctx, http.MethodGet, ipSetPath(projectUUID, uuid)+"members/", nil, &members); err != nil {
		return nil, err
	}
	return members, nil
}

func (c *Client) GetIPSetMember(ctx context.Context, projectUUID, uuid, cidr string) (*IPSetMember, error) {
	members, err := c.ListIPSetMembers(ctx, projectUUID, uuid)
	if err != nil {
		return nil, err
	}
	for i := range members {
		if members[i].CIDR == cidr {
			return &members[i], nil
		}
	}
	return nil, &Error{Status: http.StatusNotFound, Body: fmt.Sprintf("IP set member %s not found", cidr)}
}

func (c *Client) CreateIPSetMember(ctx context.Context, projectUUID, uuid string, member IPSetMember) error {
	return c.do(ctx, http.MethodPost, ipSetPath(projectUUID, uuid)+"members/", member, nil)
}

func (c *Client) UpdateIPSetMember(ctx context.Context, projectUUID, uuid string, member IPSetMember) error {
	return c.do(ctx, http.MethodPut, ipSetPath(projectUUID, uuid)+"members/", member, nil)
}

func (c *Client) DeleteIPSetMember(ctx context.Context, projectUUID, uuid, cidr string) error {
	return c.do(ctx, http.MethodDelete, ipSetPath(projectUUID, uuid)+"members/", map[string]string{"cidr": cidr}, nil)
}
