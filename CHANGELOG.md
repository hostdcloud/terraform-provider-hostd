# Changelog

## 0.1.1

- Projects, SSH keys, virtual machines, firewall IP sets, security groups, VM firewall and public IP PTR.
- Data sources for OS images, project public IPs, private networks (with free addresses) and account IP addresses.
- VM private networks use `lan_interfaces` (`auto` or `select`, with `is_gateway`).
- `power_state` and `timeouts`. The API key is checked when Terraform starts. API errors are plain text, and GET requests are retried.
- `ipv4` and `ipv6` stay known in the plan while `wan_mode` does not change. Switching between `ipv4`, `ipv6` and `dual` must go through `off`.
