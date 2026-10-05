// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type PublicIP struct {
	IP         string  `json:"ip"`
	Content    *string `json:"content"`
	Comment    *string `json:"comment"`
	VMID       *int64  `json:"vmid"`
	Status     string  `json:"status"`
	Attachment *string `json:"attachment"`
}

func publicIPPath(projectUUID, ip string) string {
	return "/cloud/project/" + projectUUID + "/public-ips/" + url.PathEscape(ip) + "/"
}

func (c *Client) ListPublicIPs(ctx context.Context, projectUUID string) ([]PublicIP, error) {
	var ips []PublicIP
	if err := c.do(ctx, http.MethodGet, "/cloud/project/"+projectUUID+"/public-ips/", nil, &ips); err != nil {
		return nil, err
	}
	return ips, nil
}

func (c *Client) GetPublicIP(ctx context.Context, projectUUID, ip string) (*PublicIP, error) {
	ips, err := c.ListPublicIPs(ctx, projectUUID)
	if err != nil {
		return nil, err
	}
	for i := range ips {
		if ips[i].IP == ip {
			return &ips[i], nil
		}
	}
	return nil, &Error{Status: http.StatusNotFound, Body: fmt.Sprintf("public IP %s not found", ip)}
}

func (c *Client) UpdatePublicIP(ctx context.Context, projectUUID, ip, content, comment string) (*PublicIP, error) {
	var updated PublicIP
	in := map[string]string{"content": content, "comment": comment}
	if err := c.do(ctx, http.MethodPatch, publicIPPath(projectUUID, ip), in, &updated); err != nil {
		return nil, err
	}
	return &updated, nil
}
