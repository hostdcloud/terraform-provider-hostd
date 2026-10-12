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
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccIPSet_basic(t *testing.T) {
	rName := "s" + acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum)
	setName := "hostd_ip_set.test"
	memberName := "hostd_ip_set_member.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckProject(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccIPSetConfig(rName, rName, "", "203.0.113.10", ""),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(setName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(setName, tfjsonpath.New("name"), knownvalue.StringExact(rName)),
					statecheck.ExpectKnownValue(setName, tfjsonpath.New("pve_ipset_name"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(memberName, tfjsonpath.New("cidr"), knownvalue.StringExact("203.0.113.10")),
					statecheck.ExpectKnownValue(memberName, tfjsonpath.New("nomatch"), knownvalue.Bool(false)),
				},
			},
			{
				Config: testAccIPSetConfig(rName, rName, "offices", "203.0.113.10", "office"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(setName, plancheck.ResourceActionUpdate),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(setName, tfjsonpath.New("comment"), knownvalue.StringExact("offices")),
					statecheck.ExpectKnownValue(memberName, tfjsonpath.New("comment"), knownvalue.StringExact("office")),
				},
			},
			{
				// An IP set cannot be renamed: a new name replaces it.
				Config: testAccIPSetConfig(rName, rName+"b", "offices", "203.0.113.10", "office"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectResourceAction(setName, plancheck.ResourceActionReplace),
					},
				},
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(setName, tfjsonpath.New("name"), knownvalue.StringExact(rName+"b")),
				},
			},
			{
				ResourceName:      setName,
				ImportState:       true,
				ImportStateIdFunc: testAccIPSetImportID(setName),
				ImportStateVerify: true,
			},
			{
				ResourceName:                         memberName,
				ImportState:                          true,
				ImportStateIdFunc:                    testAccIPSetMemberImportID(memberName),
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "cidr",
			},
		},
	})
}

func testAccIPSetImportID(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs := s.RootModule().Resources[resourceName]
		if rs == nil {
			return "", fmt.Errorf("IP set not found in state")
		}
		return rs.Primary.Attributes["project_id"] + "," + rs.Primary.ID, nil
	}
}

func testAccIPSetMemberImportID(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs := s.RootModule().Resources[resourceName]
		if rs == nil {
			return "", fmt.Errorf("IP set member not found in state")
		}
		return rs.Primary.Attributes["project_id"] + "," + rs.Primary.Attributes["ip_set_id"] + "," + rs.Primary.Attributes["cidr"], nil
	}
}

func testAccIPSetConfig(projectName, name, comment, cidr, memberComment string) string {
	return fmt.Sprintf(`
resource "hostd_project" "test" {
  name         = %[1]q
  cluster_name = %[2]q
}

resource "hostd_ip_set" "test" {
  project_id = hostd_project.test.id
  name       = %[3]q
  comment    = %[4]q
}

resource "hostd_ip_set_member" "test" {
  project_id = hostd_ip_set.test.project_id
  ip_set_id  = hostd_ip_set.test.id
  cidr       = %[5]q
  comment    = %[6]q
}
`, projectName, os.Getenv("HOSTD_TEST_CLUSTER"), name, comment, cidr, memberComment)
}
