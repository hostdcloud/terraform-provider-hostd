resource "hostd_vm_firewall_rule" "ssh" {
  project_id = hostd_vm.web.project_id
  vmid       = hostd_vm.web.vmid
  position   = 0
  type       = "in"
  action     = "ACCEPT"
  proto      = "tcp"
  dport      = "22"

  depends_on = [hostd_vm_firewall_options.web]
}

resource "hostd_vm_firewall_rule" "web" {
  project_id = hostd_vm.web.project_id
  vmid       = hostd_vm.web.vmid
  position   = 1
  type       = "group"
  action     = hostd_security_group.web.pve_group_name

  depends_on = [hostd_vm_firewall_rule.ssh]
}
