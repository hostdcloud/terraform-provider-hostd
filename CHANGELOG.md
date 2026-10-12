# Changelog

## 0.2.0

Upgrade notes:

- `hostd_security_group_rule`, `hostd_vm_firewall_rule` and `hostd_vm_firewall_options` are removed. Rules addressed by position could be applied out of order or hit the wrong rule. Move group rules into `rules` on `hostd_security_group`, and VM options and rules into one `hostd_vm_firewall` per VM. Remove the old resources from state with `terraform state rm`, then import with `terraform import hostd_security_group.<name> <project_id>,<id>` and `terraform import hostd_vm_firewall.<name> <project_id>,<vmid>`.
- `rules` is the complete list. Rules on a managed group or VM that are not in the configuration are removed on apply.
- Destroying `hostd_vm_firewall` removes every rule and turns the VM firewall off.

Changes:

- Changing `ssh_key_ids` no longer replaces the VM. Keys are installed only at create, so rotating or renaming a `hostd_ssh_key` keeps the VM.
- `hostd_ip_set`: changing `name` replaces the set (IP sets cannot be renamed); changing `comment` works in place.
- `hostd_security_group`: renaming no longer fails the apply; `pve_group_name` is updated.
- After an interrupted create, only a VM or project that appeared during the call is adopted; an existing one with the same name is not.
- The plan checks `cores`, `ram_gb`, `os_disk_gb`, `hostname`, `cpu_mode`, `wan_mode`, firewall rule fields, and that `wan_mode = "off"` has a private network.
- A paused VM is resumed or stopped to match `power_state`.
- `hostd_vm_firewall` reads unset options as the Proxmox defaults (`enable = false`, `policy_in = "DROP"`, `policy_out = "ACCEPT"`).

## 0.1.1

- Projects, SSH keys, virtual machines, firewall IP sets, security groups, VM firewall and public IP PTR.
- Data sources for OS images, project public IPs, private networks (with free addresses) and account IP addresses.
- VM private networks use `lan_interfaces` (`auto` or `select`, with `is_gateway`).
- `power_state` and `timeouts`. The API key is checked when Terraform starts. API errors are plain text, and GET requests are retried.
- `ipv4` and `ipv6` stay known in the plan while `wan_mode` does not change. Switching between `ipv4`, `ipv6` and `dual` must go through `off`.
