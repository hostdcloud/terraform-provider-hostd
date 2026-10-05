resource "hostd_security_group" "web" {
  project_id = hostd_project.main.id
  name       = "web"
  comment    = "Public web servers"
}
