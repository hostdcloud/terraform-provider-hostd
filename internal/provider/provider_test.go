// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hostdcloud/terraform-provider-hostd/internal/client"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"hostd": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("HOSTD_API_KEY") == "" {
		t.Fatal("HOSTD_API_KEY must be set for acceptance tests")
	}
}

func testAccClient() *client.Client {
	apiURL := os.Getenv("HOSTD_API_URL")
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	return client.New(apiURL, os.Getenv("HOSTD_API_KEY"), "test")
}
