// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccSecurityGroup_basic(t *testing.T) {
	rName := "g" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	groupName := "hostd_security_group.test"
	pveName := statecheck.CompareValue(compare.ValuesDiffer())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckProject(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccSecurityGroupConfig(rName, rName, `
    { type = "in", action = "ACCEPT", proto = "tcp", dport = "22" },
    { type = "in", action = "ACCEPT", proto = "tcp", dport = "443" },`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(groupName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(groupName, tfjsonpath.New("rules").AtSliceIndex(1).AtMapKey("dport"), knownvalue.StringExact("443")),
					statecheck.ExpectKnownValue(groupName, tfjsonpath.New("rules").AtSliceIndex(1).AtMapKey("enable"), knownvalue.Bool(true)),
					pveName.AddStateValue(groupName, tfjsonpath.New("pve_group_name")),
				},
			},
			{
				// Drop the first rule and add one at the end: positions shift, the list stays as written.
				Config: testAccSecurityGroupConfig(rName, rName, `
    { type = "in", action = "ACCEPT", proto = "tcp", dport = "443" },
    { type = "in", action = "DROP", proto = "icmp", comment = "no ping" },`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(groupName, tfjsonpath.New("rules"), knownvalue.ListSizeExact(2)),
					statecheck.ExpectKnownValue(groupName, tfjsonpath.New("rules").AtSliceIndex(0).AtMapKey("dport"), knownvalue.StringExact("443")),
					statecheck.ExpectKnownValue(groupName, tfjsonpath.New("rules").AtSliceIndex(1).AtMapKey("proto"), knownvalue.StringExact("icmp")),
				},
			},
			{
				// A rename changes pve_group_name in place.
				Config: testAccSecurityGroupConfig(rName, rName+"r", `
    { type = "in", action = "ACCEPT", proto = "tcp", dport = "443" },`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(groupName, plancheck.ResourceActionUpdate),
						plancheck.ExpectUnknownValue(groupName, tfjsonpath.New("pve_group_name")),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					pveName.AddStateValue(groupName, tfjsonpath.New("pve_group_name")),
				},
			},
			{
				ResourceName:      groupName,
				ImportState:       true,
				ImportStateIdFunc: testAccSecurityGroupImportID(groupName),
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

func testAccSecurityGroupConfig(projectName, name, rules string) string {
	return fmt.Sprintf(`
resource "hostd_project" "test" {
  name         = %[1]q
  cluster_name = %[2]q
}

resource "hostd_security_group" "test" {
  project_id = hostd_project.test.id
  name       = %[3]q
  rules = [%[4]s
  ]
}
`, projectName, os.Getenv("HOSTD_TEST_CLUSTER"), name, rules)
}
