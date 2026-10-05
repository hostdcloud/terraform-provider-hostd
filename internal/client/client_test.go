// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
