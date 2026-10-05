// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hostdcloud/terraform-provider-hostd/internal/client"
)

// Projects are limited per cluster, so these tests run one at a time.
func TestAccProject_basic(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	resourceName := "hostd_project.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckProject(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectConfig_basic(rName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(rName)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("description"), knownvalue.StringExact("")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("cluster_name"), knownvalue.StringExact(os.Getenv("HOSTD_TEST_CLUSTER"))),
				},
			},
			{
				Config: testAccProjectConfig_description(rName+"-renamed", "managed by terraform"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(rName+"-renamed")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("description"), knownvalue.StringExact("managed by terraform")),
				},
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccProject_disappears(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	resourceName := "hostd_project.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckProject(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccProjectConfig_basic(rName),
				Check: func(s *terraform.State) error {
					rs := s.RootModule().Resources[resourceName]
					if rs == nil {
						return fmt.Errorf("project not found in state")
					}
					return testAccClient().DeleteProject(context.Background(), rs.Primary.ID)
				},
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func testAccPreCheckProject(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)
	if os.Getenv("HOSTD_TEST_CLUSTER") == "" {
		t.Fatal("HOSTD_TEST_CLUSTER must be set to an active cluster name (e.g. waw-1) for project acceptance tests")
	}
}

func testAccCheckProjectDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "hostd_project" {
			continue
		}
		_, err := testAccClient().GetProject(context.Background(), rs.Primary.ID)
		if err == nil {
			return fmt.Errorf("project %s still exists", rs.Primary.ID)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func testAccProjectConfig_basic(rName string) string {
	return fmt.Sprintf(`
resource "hostd_project" "test" {
  name         = %[1]q
  cluster_name = %[2]q
}
`, rName, os.Getenv("HOSTD_TEST_CLUSTER"))
}

func testAccProjectConfig_description(rName, description string) string {
	return fmt.Sprintf(`
resource "hostd_project" "test" {
  name         = %[1]q
  cluster_name = %[2]q
  description  = %[3]q
}
`, rName, os.Getenv("HOSTD_TEST_CLUSTER"), description)
}
