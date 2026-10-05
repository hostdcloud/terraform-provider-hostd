resource "hostd_security_group_rule" "ssh" {
  project_id         = hostd_security_group.web.project_id
  security_group_id  = hostd_security_group.web.id
  position           = 0
  type               = "in"
  action             = "ACCEPT"
  proto              = "tcp"
  source             = "+${hostd_ip_set.office.pve_ipset_name}"
  dport              = "22"
}
