// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdatePublicIP_encodesIPv6(t *testing.T) {
	var method, path, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		_, _ = w.Write([]byte(`{"ip":"2001:db8::1","content":"vm.example.com","comment":"","vmid":null,"status":"reserved","attachment":null}`))
	}))
	defer srv.Close()

	ip, err := New(srv.URL, "test", "test").UpdatePublicIP(context.Background(), "p", "2001:db8::1", "vm.example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPatch || path != "/v1/cloud/project/p/public-ips/2001:db8::1/" {
		t.Fatalf("%s %s", method, path)
	}
	if body != `{"comment":"","content":"vm.example.com"}` {
		t.Fatalf("body %s", body)
	}
	if ip.IP != "2001:db8::1" || ip.Status != "reserved" {
		t.Fatalf("%+v", ip)
	}
}
