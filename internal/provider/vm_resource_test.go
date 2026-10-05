// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// A VM needs a project, and projects are limited per cluster, so these tests run one at a time.
func TestAccVM_basic(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	resourceName := "hostd_vm.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckVM(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMConfig(rName, 2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("vmid"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("ram_gb"), knownvalue.Int64Exact(2)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("cpu_mode"), knownvalue.StringExact("regular")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("wan_mode"), knownvalue.StringExact("dual")),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("ipv4"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("ipv6"), knownvalue.NotNull()),
				},
			},
			{
				Config: testAccVMConfig(rName, 4),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("ram_gb"), knownvalue.Int64Exact(4)),
				},
			},
			{
				ResourceName:                         resourceName,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "vmid",
				ImportStateIdFunc:                    testAccVMImportID(resourceName),
				ImportStateVerifyIgnore:              []string{"os_image", "ssh_key_ids", "username"},
			},
		},
	})
}

func TestAccVM_disappears(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	resourceName := "hostd_vm.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckVM(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccVMConfig(rName, 2),
				Check: func(s *terraform.State) error {
					rs := s.RootModule().Resources[resourceName]
					if rs == nil {
						return fmt.Errorf("VM not found in state")
					}
					vmid, err := strconv.ParseInt(rs.Primary.Attributes["vmid"], 10, 64)
					if err != nil {
						return err
					}
					return testAccClient().DestroyVM(context.Background(), rs.Primary.Attributes["project_id"], vmid, true)
				},
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func testAccPreCheckVM(t *testing.T) {
	t.Helper()
	testAccPreCheckProject(t)
	if os.Getenv("HOSTD_TEST_OS_IMAGE") == "" {
		t.Fatal("HOSTD_TEST_OS_IMAGE must be set to an image name from the project (e.g. debian-12-amd64.qcow2) for VM acceptance tests")
	}
}

func testAccVMImportID(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		rs := s.RootModule().Resources[resourceName]
		if rs == nil {
			return "", fmt.Errorf("VM not found in state")
		}
		return rs.Primary.Attributes["project_id"] + "/" + rs.Primary.Attributes["vmid"], nil
	}
}

func testAccVMConfig(rName string, ramGB int) string {
	return fmt.Sprintf(`
resource "hostd_project" "test" {
  name         = %[1]q
  cluster_name = %[2]q
}

resource "hostd_ssh_key" "test" {
  name           = %[1]q
  public_ssh_key = %[4]q
}

resource "hostd_vm" "test" {
  project_id  = hostd_project.test.id
  hostname    = %[1]q
  os_image    = %[3]q
  ssh_key_ids = [hostd_ssh_key.test.id]
  cores       = 2
  ram_gb      = %[5]d
  os_disk_gb  = 20
}
`, rName, os.Getenv("HOSTD_TEST_CLUSTER"), os.Getenv("HOSTD_TEST_OS_IMAGE"), testAccSSHPublicKey, ramGB)
}
