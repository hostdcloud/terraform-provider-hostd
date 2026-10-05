// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

// Package client is a minimal client for the public hostd API (https://api.hostd.cloud/v1).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL   string
	apiKey    string
	userAgent string
	http      *http.Client
}

// New returns a client for the API at apiURL (without the /v1 suffix).
func New(apiURL, apiKey, version string) *Client {
	return &Client{
		baseURL: strings.TrimRight(apiURL, "/") + "/v1",
		apiKey:  apiKey,
		// The API edge rejects Go's default User-Agent with an empty 403.
		userAgent: "terraform-provider-hostd/" + version,
		// Create, resize and delete calls are synchronous and can take minutes.
		http: &http.Client{Timeout: 3 * time.Minute},
	}
}

// Error is a non-2xx API response.
type Error struct {
	Status int
	Body   string
}

func (e *Error) Error() string {
	return fmt.Sprintf("hostd API returned %d: %s", e.Status, e.Body)
}

func IsNotFound(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

const maxRetries = 10

// do sends the request and retries on 429 after the Retry-After delay. A 429
// without Retry-After (e.g. the Backup now limit) is returned as an error.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var payload []byte
	if in != nil {
		var err error
		if payload, err = json.Marshal(in); err != nil {
			return err
		}
	}

	for attempt := 0; ; attempt++ {
		resp, data, err := c.send(ctx, method, path, payload)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds >= 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(seconds) * time.Second):
				}
				continue
			}
		}
		if resp.StatusCode >= http.StatusMultipleChoices {
			return &Error{Status: resp.StatusCode, Body: strings.TrimSpace(string(data))}
		}
		if out != nil {
			return json.Unmarshal(data, out)
		}
		return nil
	}
}

func (c *Client) send(ctx context.Context, method, path string, payload []byte) (*http.Response, []byte, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	return resp, data, err
}
