// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDestroyVM(t *testing.T) {
	vmPollInterval = time.Millisecond

	tests := []struct {
		name     string
		statuses []string // GET responses in order
		want     []string
	}{
		{
			name:     "running VM is hard-stopped, awaited, then deleted",
			statuses: []string{"running", "running", "stopped"},
			want:     []string{"GET", "POST /power/stop/", "GET", "GET", "DELETE true"},
		},
		{
			name:     "stopped VM is deleted right away",
			statuses: []string{"stopped"},
			want:     []string{"GET", "DELETE true"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var calls []string
			gets := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch {
				case r.Method == http.MethodGet:
					calls = append(calls, "GET")
					status := tt.statuses[min(gets, len(tt.statuses)-1)]
					gets++
					_, _ = w.Write([]byte(`{"status":"` + status + `"}`))
				case r.Method == http.MethodPost && r.URL.Path == "/v1/cloud/project/p/vm/7/power/stop/":
					calls = append(calls, "POST /power/stop/")
					_, _ = w.Write([]byte(`{}`))
				case r.Method == http.MethodDelete && r.URL.Path == "/v1/cloud/project/p/vm/7/":
					calls = append(calls, "DELETE "+r.URL.Query().Get("release_public_ip"))
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				}
			}))
			defer srv.Close()

			if err := New(srv.URL, "test", "test").DestroyVM(context.Background(), "p", 7, true); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, tt.want) {
				t.Fatalf("calls %v, want %v", calls, tt.want)
			}
		})
	}
}

func TestDestroyVM_keepsPublicIPs(t *testing.T) {
	vmPollInterval = time.Millisecond
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"status":"stopped"}`))
		case http.MethodDelete:
			query = r.URL.Query().Get("release_public_ip")
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	if err := New(srv.URL, "test", "test").DestroyVM(context.Background(), "p", 7, false); err != nil {
		t.Fatal(err)
	}
	if query != "false" {
		t.Fatalf("release_public_ip %q, want false", query)
	}
}

// interruptedCreate serves list before the POST, drops the POST connection,
// then serves listAfter.
func interruptedCreate(t *testing.T, listBefore, listAfter string) *httptest.Server {
	posted := false
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posted = true
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
		if posted {
			_, _ = w.Write([]byte(listAfter))
			return
		}
		_, _ = w.Write([]byte(listBefore))
	}))
}

func TestCreateVM_adoptsAfterInterrupt(t *testing.T) {
	t.Run("adopts the new VM with the hostname", func(t *testing.T) {
		srv := interruptedCreate(t, `[{"vmid":3,"name":"db"}]`, `[{"vmid":3,"name":"db"},{"vmid":9,"name":"web"}]`)
		defer srv.Close()

		vmid, err := New(srv.URL, "test", "test").CreateVM(context.Background(), "p", CreateVMInput{Hostname: "web"})
		if err != nil || vmid != 9 {
			t.Fatalf("got vmid %d err %v, want 9", vmid, err)
		}
	})

	t.Run("a VM that already had the hostname is not adopted", func(t *testing.T) {
		srv := interruptedCreate(t, `[{"vmid":4,"name":"web"}]`, `[{"vmid":4,"name":"web"}]`)
		defer srv.Close()

		vmid, err := New(srv.URL, "test", "test").CreateVM(context.Background(), "p", CreateVMInput{Hostname: "web"})
		if err == nil || vmid != 0 {
			t.Fatalf("got vmid %d err %v, want the interruption error", vmid, err)
		}
	})

	t.Run("an API error is not a lookup", func(t *testing.T) {
		var gets int
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				gets++
				_, _ = w.Write([]byte(`[]`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"hostname":["invalid"]}`))
		}))
		defer srv.Close()

		_, err := New(srv.URL, "test", "test").CreateVM(context.Background(), "p", CreateVMInput{Hostname: "web"})
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest || gets != 1 {
			t.Fatalf("got err %v after %d GETs, want a 400 and only the list before create", err, gets)
		}
	})

	t.Run("two new VMs with the hostname is an error", func(t *testing.T) {
		srv := interruptedCreate(t, `[]`, `[{"vmid":1,"name":"web"},{"vmid":2,"name":"web"}]`)
		defer srv.Close()

		_, err := New(srv.URL, "test", "test").CreateVM(context.Background(), "p", CreateVMInput{Hostname: "web"})
		if err == nil || !strings.Contains(err.Error(), "2 new VMs") {
			t.Fatalf("got %v", err)
		}
	})
}
