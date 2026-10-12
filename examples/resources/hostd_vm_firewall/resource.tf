resource "hostd_vm_firewall" "web" {
  project_id = hostd_vm.web.project_id
  vmid       = hostd_vm.web.vmid
  enable     = true
  policy_in  = "DROP"
  policy_out = "ACCEPT"

  # Evaluated top to bottom. Terraform keeps exactly this list on the VM.
  rules = [
    { type = "group", action = hostd_security_group.web.pve_group_name },
    { type = "in", action = "ACCEPT", proto = "icmp", comment = "ping" },
  ]
}
