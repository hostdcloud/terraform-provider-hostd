// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hostdcloud/terraform-provider-hostd/internal/client"
)

const testAccSSHPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHSpeie4eilcueJrVgfWNAcKimZbVS3XMBVGwhcHpsqh test@hostd"

func TestAccSSHKey_basic(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	resourceName := "hostd_ssh_key.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSHKeyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccSSHKeyConfig_basic(rName, testAccSSHPublicKey),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("name"), knownvalue.StringExact(rName)),
					statecheck.ExpectKnownValue(resourceName, tfjsonpath.New("public_ssh_key"), knownvalue.StringExact(testAccSSHPublicKey)),
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

// The API trims whitespace; a key read with file() ends in a newline and must not cause a diff.
func TestAccSSHKey_trailingNewline(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSHKeyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccSSHKeyConfig_basic(rName, testAccSSHPublicKey+"\n"),
			},
		},
	})
}

func TestAccSSHKey_disappears(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)
	resourceName := "hostd_ssh_key.test"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSSHKeyDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccSSHKeyConfig_basic(rName, testAccSSHPublicKey),
				Check: func(s *terraform.State) error {
					id, err := testAccSSHKeyID(s.RootModule().Resources[resourceName])
					if err != nil {
						return err
					}
					return testAccClient().DeleteSSHKey(context.Background(), id)
				},
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func testAccSSHKeyID(rs *terraform.ResourceState) (int64, error) {
	if rs == nil {
		return 0, fmt.Errorf("SSH key not found in state")
	}
	return strconv.ParseInt(rs.Primary.Attributes["id"], 10, 64)
}

func testAccCheckSSHKeyDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "hostd_ssh_key" {
			continue
		}
		id, err := testAccSSHKeyID(rs)
		if err != nil {
			return err
		}
		_, err = testAccClient().GetSSHKey(context.Background(), id)
		if err == nil {
			return fmt.Errorf("SSH key %d still exists", id)
		}
		if !client.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func testAccSSHKeyConfig_basic(rName, publicKey string) string {
	return fmt.Sprintf(`
resource "hostd_ssh_key" "test" {
  name           = %[1]q
  public_ssh_key = %[2]q
}
`, rName, publicKey)
}
