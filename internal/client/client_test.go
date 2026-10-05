// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDo_retryOn429(t *testing.T) {
	t.Run("retries after Retry-After and resends the body", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var in SSHKey
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Name != "k" {
				t.Errorf("request %d: unexpected body (name %q, err %v)", hits.Load()+1, in.Name, err)
			}
			if hits.Add(1) < 3 {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(SSHKey{ID: 7, Name: in.Name})
		}))
		defer srv.Close()

		key, err := New(srv.URL, "test", "test").CreateSSHKey(context.Background(), "k", "ssh-ed25519 AAAA")
		if err != nil {
			t.Fatal(err)
		}
		if key.ID != 7 || hits.Load() != 3 {
			t.Fatalf("got id %d after %d requests, want id 7 after 3", key.ID, hits.Load())
		}
	})

	t.Run("429 without Retry-After is an error", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()

		_, err := New(srv.URL, "test", "test").ListSSHKeys(context.Background())
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusTooManyRequests || hits.Load() != 1 {
			t.Fatalf("got err %v after %d requests, want a 429 Error after 1", err, hits.Load())
		}
	})

	t.Run("stops waiting when the context ends", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err := New(srv.URL, "test", "test").ListSSHKeys(ctx)
		if !errors.Is(err, context.DeadlineExceeded) || hits.Load() != 1 {
			t.Fatalf("got err %v after %d requests, want DeadlineExceeded after 1", err, hits.Load())
		}
	})
}

// The API edge answers Go's default User-Agent with an empty 403.
func TestDo_sendsOwnUserAgent(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	if _, err := New(srv.URL, "test", "1.2.3").ListSSHKeys(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got != "terraform-provider-hostd/1.2.3" {
		t.Fatalf("User-Agent %q, want terraform-provider-hostd/1.2.3", got)
	}
}

func TestDo_retryGET(t *testing.T) {
	retryBase = 0

	t.Run("retries 500 then succeeds", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if hits.Add(1) < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"detail":"The cluster could not complete the request"}`))
				return
			}
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()

		if _, err := New(srv.URL, "test", "test").ListSSHKeys(context.Background()); err != nil {
			t.Fatal(err)
		}
		if hits.Load() != 3 {
			t.Fatalf("got %d requests, want 3", hits.Load())
		}
	})

	t.Run("does not retry a POST 500", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"detail":"failed"}`))
		}))
		defer srv.Close()

		_, err := New(srv.URL, "test", "test").CreateSSHKey(context.Background(), "k", "ssh-ed25519 AAAA")
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusInternalServerError || hits.Load() != 1 {
			t.Fatalf("got err %v after %d requests, want a 500 Error after 1", err, hits.Load())
		}
		if !strings.Contains(err.Error(), "failed") || strings.Contains(err.Error(), `"detail"`) {
			t.Fatalf("error %q should be the detail text", err)
		}
	})

	t.Run("retries a dropped connection", func(t *testing.T) {
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if hits.Add(1) == 1 {
				hj, ok := w.(http.Hijacker)
				if !ok {
					t.Fatal("cannot hijack")
				}
				conn, _, err := hj.Hijack()
				if err != nil {
					t.Fatal(err)
				}
				conn.Close()
				return
			}
			_, _ = w.Write([]byte(`[]`))
		}))
		defer srv.Close()

		if _, err := New(srv.URL, "test", "test").ListSSHKeys(context.Background()); err != nil {
			t.Fatal(err)
		}
		if hits.Load() != 2 {
			t.Fatalf("got %d requests, want 2", hits.Load())
		}
	})

	t.Run("stops when the retry budget is already spent", func(t *testing.T) {
		previous := retryBudget
		retryBudget = 0
		t.Cleanup(func() { retryBudget = previous })

		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := New(srv.URL, "test", "test").ListSSHKeys(context.Background())
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusInternalServerError || hits.Load() != 1 {
			t.Fatalf("got err %v after %d requests, want a 500 Error after 1", err, hits.Load())
		}
	})
}

func TestFormatBody(t *testing.T) {
	tests := []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusForbidden, "", "empty 403 from the API edge (blocked User-Agent, geo-block, or the key has no access)"},
		{http.StatusUnauthorized, `{"detail":"Invalid API key."}`, "Invalid API key."},
		{http.StatusBadRequest, `{"cluster_name":["This field is required."]}`, "cluster_name: This field is required."},
		{http.StatusBadRequest, `{"cores":["even"],"hostname":["invalid"]}`, "cores: even; hostname: invalid"},
		{http.StatusForbidden, `{"error":"Verify your card.","code":"payment_verification_required"}`, "Verify your card."},
		{http.StatusInternalServerError, "not json", "not json"},
	}
	for _, tt := range tests {
		if got := formatBody(tt.status, tt.body); got != tt.want {
			t.Errorf("formatBody(%d, %q) = %q, want %q", tt.status, tt.body, got, tt.want)
		}
	}
}

func TestValidateAPIURL(t *testing.T) {
	if err := ValidateAPIURL("https://api.hostd.cloud"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAPIURL("https://api.hostd.cloud/"); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"api.hostd.cloud", "https://api.hostd.cloud/v1", "ftp://api.hostd.cloud"} {
		if err := ValidateAPIURL(raw); err == nil {
			t.Errorf("ValidateAPIURL(%q) succeeded", raw)
		}
	}
}

func TestCheckAccess(t *testing.T) {
	retryBase = 0

	t.Run("401 is an invalid key", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/v1/cloud/account-usage/" {
				t.Errorf("path %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"detail":"Invalid API key."}`))
		}))
		defer srv.Close()

		err := New(srv.URL, "test", "test").CheckAccess(context.Background())
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusUnauthorized || !strings.Contains(err.Error(), "invalid API key") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("200 is success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"limits":{},"used":{}}`))
		}))
		defer srv.Close()

		if err := New(srv.URL, "test", "test").CheckAccess(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCreateProject_adoptsExistingAfterInterrupt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("cannot hijack")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			conn.Close()
			return
		}
		_, _ = w.Write([]byte(`[{"uuid":"p1","name":"prod","description":"","cluster":"waw-1"},{"uuid":"p2","name":"prod","description":"","cluster":"waw-2"}]`))
	}))
	defer srv.Close()

	project, err := New(srv.URL, "test", "test").CreateProject(context.Background(), "waw-1", "prod", "")
	if err != nil || project.UUID != "p1" {
		t.Fatalf("got %+v err %v, want p1", project, err)
	}
}

func TestDo_annotatesAccessAndQuota(t *testing.T) {
	t.Run("read-only key", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"This API key has read-only access to this project."}`))
		}))
		defer srv.Close()

		_, err := New(srv.URL, "test", "test").ListSSHKeys(context.Background())
		if err == nil || !strings.Contains(err.Error(), "apply needs Write") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("auditor role", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"Insufficient permissions for this action."}`))
		}))
		defer srv.Close()

		_, err := New(srv.URL, "test", "test").ListSSHKeys(context.Background())
		if err == nil || !strings.Contains(err.Error(), "auditor role") {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("quota includes account usage", func(t *testing.T) {
		var usageHits int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/account-usage/") {
				usageHits++
				_, _ = w.Write([]byte(`{"limits":{"max_cores":16,"max_cores_per_vm":8},"used":{"cores":14,"ram_gb":32,"storage_fast_gb":200}}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"cores":["Account CPU limit reached."]}`))
		}))
		defer srv.Close()

		_, err := New(srv.URL, "test", "test").CreateSSHKey(context.Background(), "k", "ssh-ed25519 AAAA")
		if err == nil || !strings.Contains(err.Error(), "14 of 16 vCPU") || usageHits != 1 {
			t.Fatalf("got %v after %d usage reads", err, usageHits)
		}
	})
}
