resource "hostd_vm_firewall_options" "web" {
  project_id = hostd_vm.web.project_id
  vmid       = hostd_vm.web.vmid
  enable     = true
  policy_in  = "DROP"
  policy_out = "ACCEPT"
}
