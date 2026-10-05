// Copyright (c) 2026 hostd.cloud
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccOSImagesDataSource_basic(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckVM(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckProjectDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccOSImagesDataSourceConfig(rName),
				Check:  resource.TestCheckTypeSetElemAttr("data.hostd_os_images.test", "names.*", os.Getenv("HOSTD_TEST_OS_IMAGE")),
			},
		},
	})
}

func testAccOSImagesDataSourceConfig(rName string) string {
	return fmt.Sprintf(`
resource "hostd_project" "test" {
  name         = %[1]q
  cluster_name = %[2]q
}

data "hostd_os_images" "test" {
  project_id = hostd_project.test.id
}
`, rName, os.Getenv("HOSTD_TEST_CLUSTER"))
}
