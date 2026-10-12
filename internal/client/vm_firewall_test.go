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
	group := []FirewallRuleInput{{Type: "group", Action: "web_a3", Enable: true}}
	if err := c.ReplaceVMFirewallRules(context.Background(), "p", 7, group); err != nil {
		t.Fatal(err)
	}
	rule := body["rules"].([]any)[0].(map[string]any)
	if method != http.MethodPut || path != "/v1/cloud/project/p/vm/7/firewall/rules/replace/" || rule["action"] != "web_a3" {
		t.Fatalf("replace %s %s %v", method, path, body)
	}
	if _, ok := rule["proto"]; ok {
		t.Fatalf("group rule must not send proto: %v", rule)
	}
}
