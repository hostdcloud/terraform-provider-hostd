resource "hostd_security_group" "web" {
  project_id = hostd_project.main.id
  name       = "web"
  comment    = "Public web servers"

  rules = [
    { type = "in", action = "ACCEPT", proto = "tcp", dport = "80,443" },
    { type = "in", action = "ACCEPT", proto = "tcp", dport = "22", source = "+${hostd_ip_set.office.pve_ipset_name}" },
  ]
}
