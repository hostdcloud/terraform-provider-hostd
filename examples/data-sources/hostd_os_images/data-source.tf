data "hostd_os_images" "available" {
  project_id = hostd_project.main.id
}

output "os_images" {
  value = data.hostd_os_images.available.names
}
