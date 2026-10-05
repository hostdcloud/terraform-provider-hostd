// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccSecurityGroup_basic(t *testing.T) {
	rName := "g" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	groupName := "hostd_security_group.test"
	ruleName := "hostd_security_group_rule.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckProject(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccSecurityGroupConfig(rName, "22"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(groupName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(groupName, tfjsonpath.New("pve_group_name"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(ruleName, tfjsonpath.New("position"), knownvalue.Int64Exact(0)),
					statecheck.ExpectKnownValue(ruleName, tfjsonpath.New("dport"), knownvalue.StringExact("22")),
					statecheck.ExpectKnownValue(ruleName, tfjsonpath.New("enable"), knownvalue.Bool(true)),
				},
			},
			{
				Config: testAccSecurityGroupConfig(rName, "443"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(ruleName, tfjsonpath.New("dport"), knownvalue.StringExact("443")),
				},
			},
			{
				ResourceName:      groupName,
				ImportState:       true,
				ImportStateIdFunc: testAccSecurityGroupImportID(groupName),
				ImportStateVerify: true,
			},
			{
				ResourceName:      ruleName,
				ImportState:       true,
				ImportStateIdFunc: testAccSecurityGroupRuleImportID(ruleName),
				ImportStateVerify: true,
			},
		},
	})
}

func testAccSecurityGroupImportID(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs := s.RootModule().Resources[resourceName]
		if rs == nil {
			return "", fmt.Errorf("security group not found in state")
		}
		return rs.Primary.Attributes["project_id"] + "," + rs.Primary.ID, nil
	}
}

func testAccSecurityGroupRuleImportID(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs := s.RootModule().Resources[resourceName]
		if rs == nil {
			return "", fmt.Errorf("security group rule not found in state")
		}
		return rs.Primary.Attributes["project_id"] + "," + rs.Primary.Attributes["security_group_id"] + "," + rs.Primary.Attributes["position"], nil
	}
}

func testAccSecurityGroupConfig(name, dport string) string {
	return fmt.Sprintf(`
resource "hostd_project" "test" {
  name         = %[1]q
  cluster_name = %[2]q
}

resource "hostd_security_group" "test" {
  project_id = hostd_project.test.id
  name       = %[1]q
}

resource "hostd_security_group_rule" "test" {
  project_id        = hostd_security_group.test.project_id
  security_group_id = hostd_security_group.test.id
  position          = 0
  type              = "in"
  action            = "ACCEPT"
  proto             = "tcp"
  dport             = %[3]q
}
`, name, os.Getenv("HOSTD_TEST_CLUSTER"), dport)
}
