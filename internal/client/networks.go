// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"net/http"
	"net/url"
)

type PrivateNetwork struct {
	VNet          *string `json:"vnet"`
	Type          string  `json:"type"`
	LANIdentifier int64   `json:"lan_identifier"`
	CIDR          *string `json:"cidr"`
	Gateway       *string `json:"gateway"`
	HasRouter     bool    `json:"has_router"`
}

type PrivateNetworkIP struct {
	IP   string `json:"ip"`
	Free bool   `json:"free"`
}

type AccountIP struct {
	IP           *string `json:"ip"`
	Content      *string `json:"content"`
	Comment      *string `json:"comment"`
	CanUpdatePTR bool    `json:"can_update_ptr"`
	ServiceKind  *string `json:"service_kind"`
	ServiceName  string  `json:"service_name"`
	ServiceSID   *string `json:"service_sid"`
	ServiceUUID  *string `json:"service_uuid"`
	Status       string  `json:"status"`
	CanRelease   bool    `json:"can_release"`
	VM           *struct {
		VMID int64   `json:"vmid"`
		Name *string `json:"name"`
	} `json:"vm"`
}

func (c *Client) ListPrivateNetworks(ctx context.Context, projectUUID string) ([]PrivateNetwork, error) {
	var networks []PrivateNetwork
	if err := c.do(ctx, http.MethodGet, "/cloud/project/"+projectUUID+"/sdn/", nil, &networks); err != nil {
		return nil, err
	}
	return networks, nil
}

func (c *Client) ListPrivateNetworkIPs(ctx context.Context, projectUUID, vnet string) ([]PrivateNetworkIP, error) {
	var ips []PrivateNetworkIP
	path := "/cloud/project/" + projectUUID + "/sdn/private-network/vnet/" + url.PathEscape(vnet) + "/ips/"
	if err := c.do(ctx, http.MethodGet, path, nil, &ips); err != nil {
		return nil, err
	}
	return ips, nil
}

func (c *Client) ListAccountIPs(ctx context.Context) ([]AccountIP, error) {
	var ips []AccountIP
	if err := c.do(ctx, http.MethodGet, "/network/ips/", nil, &ips); err != nil {
		return nil, err
	}
	return ips, nil
}
