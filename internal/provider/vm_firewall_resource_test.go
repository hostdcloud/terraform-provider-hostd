// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/compare"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccVMFirewall_basic(t *testing.T) {
	rName := "fw" + acctest.RandStringFromCharSet(8, acctest.CharSetAlpha)
	firewallName := "hostd_vm_firewall.test"
	groupRules := `
    { type = "group", action = hostd_security_group.web.pve_group_name },
    { type = "in", action = "ACCEPT", proto = "tcp", dport = "22" },`

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckVM(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMFirewallConfig(rName, "web", `
    { type = "in", action = "ACCEPT", proto = "tcp", dport = "22" },`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(firewallName, tfjsonpath.New("enable"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(firewallName, tfjsonpath.New("policy_in"), knownvalue.StringExact("DROP")),
					statecheck.ExpectKnownValue(firewallName, tfjsonpath.New("rules"), knownvalue.ListSizeExact(1)),
				},
			},
			{
				Config: testAccVMFirewallConfig(rName, "web", groupRules),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(firewallName, tfjsonpath.New("rules").AtSliceIndex(0).AtMapKey("type"), knownvalue.StringExact("group")),
					statecheck.ExpectKnownValue(firewallName, tfjsonpath.New("rules").AtSliceIndex(1).AtMapKey("dport"), knownvalue.StringExact("22")),
				},
			},
			{
				// Renaming an attached group keeps the VM rule pointing at it.
				Config: testAccVMFirewallConfig(rName, "webr", groupRules),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.CompareValuePairs(
						firewallName, tfjsonpath.New("rules").AtSliceIndex(0).AtMapKey("action"),
						"hostd_security_group.web", tfjsonpath.New("pve_group_name"),
						compare.ValuesSame(),
					),
				},
			},
			{
				ResourceName:                         firewallName,
				ImportState:                          true,
				ImportStateIdFunc:                    testAccVMFirewallImportID(firewallName),
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "vmid",
			},
		},
	})
}

func testAccVMFirewallImportID(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs := s.RootModule().Resources[resourceName]
		if rs == nil {
			return "", fmt.Errorf("VM firewall not found in state")
		}
		return rs.Primary.Attributes["project_id"] + "," + rs.Primary.Attributes["vmid"], nil
	}
}

func testAccVMFirewallConfig(name, groupName, rules string) string {
	return testAccVMConfig(name, 2) + fmt.Sprintf(`
resource "hostd_security_group" "web" {
  project_id = hostd_project.test.id
  name       = %[1]q
  rules = [
    { type = "in", action = "ACCEPT", proto = "tcp", dport = "80,443" },
  ]
}

resource "hostd_vm_firewall" "test" {
  project_id = hostd_vm.test.project_id
  vmid       = hostd_vm.test.vmid
  enable     = true
  policy_in  = "DROP"
  policy_out = "ACCEPT"
  rules = [%[2]s
  ]
}
`, groupName, rules)
}
