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
	"maps"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
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
		// No Client.Timeout: the resource timeouts block sets the deadline.
		// send applies defaultRequestTimeout when the caller did not.
		http: &http.Client{},
	}
}

// Error is a non-2xx API response.
type Error struct {
	Status int
	Body   string
	Hint   string
}

func (e *Error) Error() string {
	msg := formatBody(e.Status, e.Body)
	if e.Hint != "" {
		msg += " " + e.Hint
	}
	return fmt.Sprintf("hostd API returned %d: %s", e.Status, msg)
}

func (c *Client) apiError(ctx context.Context, path string, status int, data []byte) *Error {
	err := &Error{Status: status, Body: strings.TrimSpace(string(data))}
	if path == "/cloud/account-usage/" {
		return err
	}
	if hint := accessHint(err.Body); hint != "" {
		err.Hint = hint
		return err
	}
	if !quotaLimited(err.Body) {
		return err
	}
	var usage accountUsage
	if c.do(ctx, http.MethodGet, "/cloud/account-usage/", nil, &usage) == nil {
		err.Hint = usage.hint()
	}
	return err
}

type accountUsage struct {
	Limits struct {
		MaxCores      int64 `json:"max_cores"`
		MaxCoresPerVM int64 `json:"max_cores_per_vm"`
	} `json:"limits"`
	Used struct {
		Cores         int64 `json:"cores"`
		RAMGB         int64 `json:"ram_gb"`
		StorageFastGB int64 `json:"storage_fast_gb"`
	} `json:"used"`
}

func (u accountUsage) hint() string {
	return fmt.Sprintf(
		"Account usage: %d of %d vCPU, %d GiB RAM, %d GiB disk; max %d vCPU per VM.",
		u.Used.Cores, u.Limits.MaxCores, u.Used.RAMGB, u.Used.StorageFastGB, u.Limits.MaxCoresPerVM,
	)
}

func accessHint(body string) string {
	switch {
	case strings.Contains(body, "read-only access to this project"):
		return "Read access refreshes state. apply needs Write on the project; destroy needs Full."
	case strings.Contains(body, "Insufficient permissions for this action"):
		return "The auditor role on a shared project can refresh state but cannot change resources."
	default:
		return ""
	}
}

func quotaLimited(body string) bool {
	for _, phrase := range []string{
		"Account CPU limit reached",
		"Max cores per VM",
		"Project RAM limit reached",
		"Project storage limit reached",
		"project limit for this cluster has been reached",
	} {
		if strings.Contains(body, phrase) {
			return true
		}
	}
	return false
}

func IsNotFound(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

// isInterrupted reports a timeout or a dropped connection. The request may
// have succeeded on the API, so the caller looks the resource up before failing.
func isInterrupted(err error) bool {
	var apiErr *Error
	if err == nil || errors.As(err, &apiErr) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr)
}

const (
	maxRetries            = 10
	defaultRequestTimeout = 3 * time.Minute
)

// retryBudget is the longest the client waits between retries of one call.
// retryBase is the first delay for a retried GET; tests set it to 0.
var (
	retryBudget = 2 * time.Minute
	retryBase   = 200 * time.Millisecond
)

// do sends the request. Every method retries 429 after Retry-After. GET also
// retries HTTP 5xx and network errors. A 429 without Retry-After (e.g. the
// Backup now limit) is returned as an error. Retries stop at maxRetries or
// retryBudget, whichever comes first.
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var payload []byte
	if in != nil {
		var err error
		if payload, err = json.Marshal(in); err != nil {
			return err
		}
	}

	deadline := time.Now().Add(retryBudget)
	for attempt := 0; ; attempt++ {
		resp, data, err := c.send(ctx, method, path, payload)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if method == http.MethodGet {
				if wait, ok := retryDelay(attempt, deadline); ok {
					if err := pause(ctx, wait); err != nil {
						return err
					}
					continue
				}
			}
			return err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries {
			if seconds, convErr := strconv.Atoi(resp.Header.Get("Retry-After")); convErr == nil && seconds >= 0 {
				wait := time.Duration(seconds)*time.Second + jitter(time.Duration(seconds)*time.Second)
				if wait > time.Until(deadline) {
					return &Error{Status: resp.StatusCode, Body: strings.TrimSpace(string(data))}
				}
				if err := pause(ctx, wait); err != nil {
					return err
				}
				continue
			}
		}
		if method == http.MethodGet && resp.StatusCode >= http.StatusInternalServerError {
			if wait, ok := retryDelay(attempt, deadline); ok {
				if err := pause(ctx, wait); err != nil {
					return err
				}
				continue
			}
		}
		if resp.StatusCode >= http.StatusMultipleChoices {
			return c.apiError(ctx, path, resp.StatusCode, data)
		}
		if out != nil {
			return json.Unmarshal(data, out)
		}
		return nil
	}
}

func retryDelay(attempt int, deadline time.Time) (time.Duration, bool) {
	if attempt >= maxRetries || !time.Now().Before(deadline) {
		return 0, false
	}
	wait := backoff(attempt)
	if wait > time.Until(deadline) {
		return 0, false
	}
	return wait, true
}

func backoff(attempt int) time.Duration {
	if retryBase <= 0 {
		return 0
	}
	shift := attempt
	if shift > 6 {
		shift = 6
	}
	d := retryBase << shift
	if d > 10*time.Second {
		d = 10 * time.Second
	}
	return d + jitter(d)
}

func jitter(d time.Duration) time.Duration {
	span := d / 5
	if span > 250*time.Millisecond {
		span = 250 * time.Millisecond
	}
	if span < time.Millisecond {
		return 0
	}
	return time.Duration(rand.Int64N(int64(span)))
}

func pause(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// ValidateAPIURL reports whether apiURL is an absolute http(s) URL with no
// path. The client appends /v1 itself.
func ValidateAPIURL(apiURL string) error {
	u, err := url.Parse(apiURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Opaque != "" {
		return fmt.Errorf("api_url must be an absolute http or https URL, got %q", apiURL)
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("api_url is the API host without a path (the client adds /v1), got %q", apiURL)
	}
	return nil
}

// CheckAccess calls account usage so an invalid key fails before any resource.
func (c *Client) CheckAccess(ctx context.Context) error {
	err := c.do(ctx, http.MethodGet, "/cloud/account-usage/", nil, nil)
	if err == nil {
		return nil
	}
	var apiErr *Error
	if errors.As(err, &apiErr) {
		switch apiErr.Status {
		case http.StatusUnauthorized:
			return fmt.Errorf("invalid API key: %w", err)
		case http.StatusForbidden:
			return fmt.Errorf("API key has no access: %w", err)
		}
	}
	return fmt.Errorf("checking API key: %w", err)
}

// formatBody turns a DRF JSON body into "detail" / "error" text or
// "field: message" pairs. An empty 403 is the API edge refusing the client.
func formatBody(status int, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		if status == http.StatusForbidden {
			return "empty 403 from the API edge (blocked User-Agent, geo-block, or the key has no access)"
		}
		return ""
	}
	var raw map[string]any
	if json.Unmarshal([]byte(body), &raw) != nil || len(raw) == 0 {
		return body
	}

	var parts []string
	if s, ok := raw["detail"].(string); ok && s != "" {
		parts = append(parts, s)
		delete(raw, "detail")
	}
	if s, ok := raw["error"].(string); ok && s != "" {
		parts = append(parts, s)
		delete(raw, "error")
	}
	delete(raw, "code")
	delete(raw, "onboarding_path")

	for _, key := range slices.Sorted(maps.Keys(raw)) {
		parts = append(parts, key+": "+flatten(raw[key]))
	}
	if len(parts) == 0 {
		return body
	}
	return strings.Join(parts, "; ")
}

func flatten(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			parts = append(parts, flatten(item))
		}
		return strings.Join(parts, ", ")
	default:
		encoded, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(encoded)
	}
}

func (c *Client) send(ctx context.Context, method, path string, payload []byte) (*http.Response, []byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultRequestTimeout)
		defer cancel()
	}

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
