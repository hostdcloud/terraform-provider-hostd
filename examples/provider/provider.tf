terraform {
  required_providers {
    hostd = {
      source = "hostdcloud/hostd"
    }
  }
}

# Without api_key the provider reads HOSTD_API_KEY from the environment.
provider "hostd" {
  api_key = var.hostd_api_key
}

variable "hostd_api_key" {
  type      = string
  sensitive = true
}
