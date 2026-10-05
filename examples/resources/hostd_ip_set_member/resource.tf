resource "hostd_ip_set_member" "office" {
  project_id = hostd_ip_set.office.project_id
  ip_set_id  = hostd_ip_set.office.id
  cidr       = "203.0.113.10"
}
