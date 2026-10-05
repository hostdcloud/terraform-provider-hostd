resource "hostd_public_ip_ptr" "web" {
  project_id = hostd_vm.web.project_id
  ip         = hostd_vm.web.ipv4
  content    = "web.example.com"
}
