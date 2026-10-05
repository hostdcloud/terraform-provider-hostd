// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
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
			want:     []string{"GET", "POST /power/stop/", "GET", "GET", "DELETE"},
		},
		{
			name:     "stopped VM is deleted right away",
			statuses: []string{"stopped"},
			want:     []string{"GET", "DELETE"},
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
					calls = append(calls, "DELETE")
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				}
			}))
			defer srv.Close()

			if err := New(srv.URL, "test", "test").DestroyVM(context.Background(), "p", 7); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(calls, tt.want) {
				t.Fatalf("calls %v, want %v", calls, tt.want)
			}
		})
	}
}
