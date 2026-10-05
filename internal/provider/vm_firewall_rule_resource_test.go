// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestVMFirewallRuleInvalid(t *testing.T) {
	group := vmFirewallRuleResourceModel{
		Type:   types.StringValue("group"),
		Action: types.StringValue("web_ab"),
		Proto:  types.StringValue("tcp"),
	}
	if group.invalid() == "" {
		t.Fatal("a group attachment must not set proto")
	}
	filter := vmFirewallRuleResourceModel{
		Type:   types.StringValue("in"),
		Action: types.StringValue("ACCEPT"),
		Proto:  types.StringValue("tcp"),
	}
	if filter.invalid() != "" {
		t.Fatal(filter.invalid())
	}
}

func TestAccVMFirewall_basic(t *testing.T) {
	rName := "fw" + acctest.RandStringFromCharSet(8, acctest.CharSetAlpha)
	optionsName := "hostd_vm_firewall_options.test"
	ruleName := "hostd_vm_firewall_rule.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckVM(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMFirewallConfig(rName, "22"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(optionsName, tfjsonpath.New("enable"), knownvalue.Bool(true)),
					statecheck.ExpectKnownValue(optionsName, tfjsonpath.New("policy_in"), knownvalue.StringExact("DROP")),
					statecheck.ExpectKnownValue(ruleName, tfjsonpath.New("dport"), knownvalue.StringExact("22")),
				},
			},
			{
				Config: testAccVMFirewallConfig(rName, "443"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(ruleName, tfjsonpath.New("dport"), knownvalue.StringExact("443")),
				},
			},
			{
				ResourceName:      optionsName,
				ImportState:       true,
				ImportStateIdFunc: testAccVMFirewallOptionsImportID(optionsName),
				ImportStateVerify: true,
			},
			{
				ResourceName:      ruleName,
				ImportState:       true,
				ImportStateIdFunc: testAccVMFirewallRuleImportID(ruleName),
				ImportStateVerify: true,
			},
		},
	})
}

func testAccVMFirewallOptionsImportID(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs := s.RootModule().Resources[resourceName]
		if rs == nil {
			return "", fmt.Errorf("firewall options not found in state")
		}
		return rs.Primary.Attributes["project_id"] + "," + rs.Primary.Attributes["vmid"], nil
	}
}

func testAccVMFirewallRuleImportID(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs := s.RootModule().Resources[resourceName]
		if rs == nil {
			return "", fmt.Errorf("firewall rule not found in state")
		}
		return rs.Primary.Attributes["project_id"] + "," + rs.Primary.Attributes["vmid"] + "," + rs.Primary.Attributes["position"], nil
	}
}

func testAccVMFirewallConfig(name, dport string) string {
	return testAccVMConfig(name, 2) + fmt.Sprintf(`
resource "hostd_vm_firewall_options" "test" {
  project_id = hostd_vm.test.project_id
  vmid       = hostd_vm.test.vmid
  enable     = true
  policy_in  = "DROP"
  policy_out = "ACCEPT"
}

resource "hostd_vm_firewall_rule" "test" {
  project_id = hostd_vm.test.project_id
  vmid       = hostd_vm.test.vmid
  position   = 0
  type       = "in"
  action     = "ACCEPT"
  proto      = "tcp"
  dport      = %[1]q

  depends_on = [hostd_vm_firewall_options.test]
}
`, dport)
}
