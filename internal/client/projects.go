// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"net/http"
)

type Project struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Cluster     string `json:"cluster"`
}

func (c *Client) GetProject(ctx context.Context, uuid string) (*Project, error) {
	var project Project
	if err := c.do(ctx, http.MethodGet, "/cloud/project/"+uuid+"/", nil, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

func (c *Client) CreateProject(ctx context.Context, clusterName, name, description string) (*Project, error) {
	var project Project
	in := map[string]string{"cluster_name": clusterName, "name": name, "description": description}
	if err := c.do(ctx, http.MethodPost, "/cloud/project/", in, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

func (c *Client) UpdateProject(ctx context.Context, uuid, name, description string) (*Project, error) {
	var project Project
	in := map[string]string{"name": name, "description": description}
	if err := c.do(ctx, http.MethodPatch, "/cloud/project/"+uuid+"/", in, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

func (c *Client) DeleteProject(ctx context.Context, uuid string) error {
	return c.do(ctx, http.MethodDelete, "/cloud/project/"+uuid+"/", nil, nil)
}
