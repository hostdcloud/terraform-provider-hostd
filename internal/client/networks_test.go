// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListPrivateNetworkIPs(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`[{"ip":"10.0.0.2","free":true},{"ip":"10.0.0.3","free":false}]`))
	}))
	defer srv.Close()

	ips, err := New(srv.URL, "test", "test").ListPrivateNetworkIPs(context.Background(), "p", "lan one")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/cloud/project/p/sdn/private-network/vnet/lan%20one/ips/" {
		t.Fatalf("path %s", path)
	}
	if len(ips) != 2 || !ips[0].Free || ips[1].Free {
		t.Fatalf("%+v", ips)
	}
}
