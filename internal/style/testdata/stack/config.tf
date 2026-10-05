terraform {
  required_version = ">= 1.11"
  backend "local" {}

  required_providers {
    kubectl = {
      version = ">= 2.4.1"
      source  = "alekc/kubectl"
    }
  }
}

provider "kubectl" {
  config_context = "admin@example"
  config_path    = "~/.kube/config"
}

variable "api_token" {
  description = "Token for the DNS provider."
  sensitive   = true
  type        = string
}

variable "replicas" {
  description = "Number of web replicas."
  default     = 2
  type        = number
}

# -------------------------------------------------------------------------------
# Network
# -------------------------------------------------------------------------------

locals {
  namespace = "web"
  domain    = "example.test"

  internal_ranges = [
    "192.0.2.0/24",
    "198.51.100.0/24"
  ]

  hosts = {
    for name, ip in local.ips : name => "${name}.${local.domain}"
  }

  # Addresses outside the cluster.
  ips = {
    storage = "192.0.2.20"
    gateway = "192.0.2.1"
    proxy   = "192.0.2.10"
  }
}
