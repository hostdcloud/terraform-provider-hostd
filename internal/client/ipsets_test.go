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

func TestDeleteIPSetMember_sendsCIDR(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/cloud/project/p/firewall/ipsets/set/members/" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			CIDR string `json:"cidr"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		got = body.CIDR
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))
	defer srv.Close()

	if err := New(srv.URL, "test", "test").DeleteIPSetMember(context.Background(), "p", "set", "203.0.113.10"); err != nil {
		t.Fatal(err)
	}
	if got != "203.0.113.10" {
		t.Fatalf("cidr %q", got)
	}
}
