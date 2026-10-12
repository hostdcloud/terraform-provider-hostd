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

func TestReplaceSecurityGroupRules_body(t *testing.T) {
	var method, path string
	var body map[string][]map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test", "test")
	rules := []FirewallRuleInput{{Type: "in", Action: "ACCEPT", Proto: "tcp", DPort: "22", Enable: true}}
	if err := c.ReplaceSecurityGroupRules(context.Background(), "p", "g", rules); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPut || path != "/v1/cloud/project/p/firewall/groups/g/rules/replace/" {
		t.Fatalf("replace %s %s", method, path)
	}
	if len(body["rules"]) != 1 || body["rules"][0]["dport"] != "22" || body["rules"][0]["enable"] != true {
		t.Fatalf("body %v", body)
	}
}

func TestReplaceSecurityGroupRules_emptyListIsSent(t *testing.T) {
	var raw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf [64]byte
		n, _ := r.Body.Read(buf[:])
		raw = string(buf[:n])
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test", "test")
	if err := c.ReplaceSecurityGroupRules(context.Background(), "p", "g", []FirewallRuleInput{}); err != nil {
		t.Fatal(err)
	}
	if raw != `{"rules":[]}` {
		t.Fatalf("body %s", raw)
	}
}
