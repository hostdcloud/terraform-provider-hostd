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

func TestSecurityGroupRule_positionBody(t *testing.T) {
	var method, path string
	var body map[string]int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		body = map[string]int64{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "test", "test")
	if err := c.DeleteSecurityGroupRule(context.Background(), "p", "g", 2); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete || path != "/v1/cloud/project/p/firewall/groups/g/rules/" || body["pos"] != 2 {
		t.Fatalf("delete %s %s %v", method, path, body)
	}
	if err := c.MoveSecurityGroupRule(context.Background(), "p", "g", 2, 0); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPost || path != "/v1/cloud/project/p/firewall/groups/g/rules/move/" || body["from_pos"] != 2 || body["to_pos"] != 0 {
		t.Fatalf("move %s %s %v", method, path, body)
	}
}
