// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
	"slices"
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

// CreateProject creates a project. If the call is interrupted, a project with
// the name and cluster that did not exist before the call is adopted.
func (c *Client) CreateProject(ctx context.Context, clusterName, name, description string) (*Project, error) {
	before, err := c.projectsByName(ctx, clusterName, name)
	if err != nil {
		return nil, err
	}
	var project Project
	in := map[string]string{"cluster_name": clusterName, "name": name, "description": description}
	err = c.do(ctx, http.MethodPost, "/cloud/project/", in, &project)
	if err == nil {
		return &project, nil
	}
	if !isInterrupted(err) {
		return nil, err
	}
	after, findErr := c.projectsByName(context.WithoutCancel(ctx), clusterName, name)
	if findErr != nil {
		return nil, fmt.Errorf("%w; looking for the project afterwards: %v", err, findErr)
	}
	created := slices.DeleteFunc(after, func(p Project) bool {
		return slices.ContainsFunc(before, func(old Project) bool { return old.UUID == p.UUID })
	})
	switch len(created) {
	case 0:
		return nil, err
	case 1:
		return &created[0], nil
	}
	return nil, fmt.Errorf("%w; %d new projects in cluster %q are named %q, import the one you want by its UUID", err, len(created), clusterName, name)
}

func (c *Client) projectsByName(ctx context.Context, clusterName, name string) ([]Project, error) {
	var projects []Project
	if err := c.do(ctx, http.MethodGet, "/cloud/project/", nil, &projects); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(projects, func(p Project) bool { return p.Name != name || p.Cluster != clusterName }), nil
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
