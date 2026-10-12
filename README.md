# terraform-provider-hostd

Terraform provider for [hostd.cloud](https://hostd.cloud) Public Cloud: projects, SSH keys, virtual machines, firewall and public IP PTR.

```hcl
terraform {
  required_providers {
    hostd = {
      source = "hostdcloud/hostd"
    }
  }
}

provider "hostd" {} # reads HOSTD_API_KEY

resource "hostd_project" "main" {
  name         = "production"
  cluster_name = "waw-1"
}
```

Resources: `hostd_project`, `hostd_ssh_key`, `hostd_vm`, `hostd_vm_firewall`, `hostd_security_group`, `hostd_ip_set`, `hostd_ip_set_member`, `hostd_public_ip_ptr`. Data sources: `hostd_os_images`, `hostd_public_ips`, `hostd_private_networks`, `hostd_ip_addresses`.
See [`docs/`](docs/) for the full reference and the API key access each resource needs.

## Development

```sh
make build     # go build
make test      # unit tests
make generate  # regenerate docs/ from the schema and examples/
make testacc   # acceptance tests, see below
```

Acceptance tests run against the real API and create billed resources. They need
`HOSTD_API_KEY` (allowed to create projects, SSH keys: write), `HOSTD_TEST_CLUSTER`
(an active cluster name) and `HOSTD_TEST_OS_IMAGE` (an image name available in the project).

## License

[MPL-2.0](LICENSE)
