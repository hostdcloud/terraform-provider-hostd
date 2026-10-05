// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVMFirewall_requests(t *testing.T) {
	var method, path string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		body = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"enable":true,"policy_in":"DROP","policy_out":"ACCEPT"}`))
			return
		}
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test", "test")
	if err := c.SetVMFirewallOptions(context.Background(), "p", 7, true, "DROP", "ACCEPT"); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPut || path != "/v1/cloud/project/p/vm/7/firewall/config/" || body["policy_in"] != "DROP" {
		t.Fatalf("set %s %s %v", method, path, body)
	}
	if err := c.DeleteVMFirewallRule(context.Background(), "p", 7, 1); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete || path != "/v1/cloud/project/p/vm/7/firewall/rules/" || body["pos"] != float64(1) {
		t.Fatalf("delete %s %s %v", method, path, body)
	}
}
