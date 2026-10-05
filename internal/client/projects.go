// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
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
	err := c.do(ctx, http.MethodPost, "/cloud/project/", in, &project)
	if err == nil {
		return &project, nil
	}
	if !isInterrupted(err) {
		return nil, err
	}
	found, findErr := c.projectByName(context.WithoutCancel(ctx), clusterName, name)
	if findErr != nil {
		return nil, findErr
	}
	if found != nil {
		return found, nil
	}
	return nil, err
}

func (c *Client) projectByName(ctx context.Context, clusterName, name string) (*Project, error) {
	var projects []Project
	if err := c.do(ctx, http.MethodGet, "/cloud/project/", nil, &projects); err != nil {
		return nil, nil
	}
	var found *Project
	matches := 0
	for i := range projects {
		if projects[i].Name == name && projects[i].Cluster == clusterName {
			matches++
			found = &projects[i]
		}
	}
	if matches > 1 {
		return nil, fmt.Errorf("%d projects in cluster %q are named %q; import the one you want by its UUID", matches, clusterName, name)
	}
	return found, nil
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
