// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"fmt"
	"net/http"
)

type SSHKey struct {
	ID           int64  `json:"id,omitempty"`
	Name         string `json:"name"`
	PublicSSHKey string `json:"public_ssh_key"`
}

func (c *Client) ListSSHKeys(ctx context.Context) ([]SSHKey, error) {
	var keys []SSHKey
	if err := c.do(ctx, http.MethodGet, "/user/ssh-keys/", nil, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// GetSSHKey finds a key by id; the API has no retrieve endpoint, so it reads the list.
func (c *Client) GetSSHKey(ctx context.Context, id int64) (*SSHKey, error) {
	keys, err := c.ListSSHKeys(ctx)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		if key.ID == id {
			return &key, nil
		}
	}
	return nil, &Error{Status: http.StatusNotFound, Body: fmt.Sprintf("SSH key %d not found", id)}
}

func (c *Client) CreateSSHKey(ctx context.Context, name, publicSSHKey string) (*SSHKey, error) {
	var key SSHKey
	in := SSHKey{Name: name, PublicSSHKey: publicSSHKey}
	if err := c.do(ctx, http.MethodPost, "/user/ssh-keys/", in, &key); err != nil {
		return nil, err
	}
	return &key, nil
}

func (c *Client) DeleteSSHKey(ctx context.Context, id int64) error {
	return c.do(ctx, http.MethodDelete, fmt.Sprintf("/user/ssh-keys/%d/", id), nil, nil)
}
