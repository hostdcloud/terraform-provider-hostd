resource "hostd_ip_set" "office" {
  project_id = hostd_project.main.id
  name       = "office"
  comment    = "Admin networks"
}
