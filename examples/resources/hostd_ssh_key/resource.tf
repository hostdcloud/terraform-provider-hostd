resource "hostd_ssh_key" "admin" {
  name           = "admin"
  public_ssh_key = file("~/.ssh/id_ed25519.pub")
}
