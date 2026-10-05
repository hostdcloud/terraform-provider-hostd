resource "hostd_vm" "web" {
  project_id  = hostd_project.main.id
  hostname    = "web-01"
  os_image    = "debian-12-amd64.qcow2" # a name from data.hostd_os_images
  ssh_key_ids = [hostd_ssh_key.admin.id]
  cores       = 2
  ram_gb      = 4
  os_disk_gb  = 20

  # Destroying this VM deletes its disks and every backup of it.
  lifecycle {
    prevent_destroy = true
  }
}

output "web_ipv4" {
  value = hostd_vm.web.ipv4
}
