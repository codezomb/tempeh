data "kubectl_file_documents" "upstream" {
  content = replace(file("${path.module}/manifests/app-v1.2.3.yaml"), "default", local.namespace)
}

resource "kubectl_manifest" "namespace" {
  yaml_body = yamlencode({
    apiVersion = "v1"
    kind       = "Namespace"

    metadata = {
      name = local.namespace

      labels = {
        "pod-security.kubernetes.io/enforce" = "baseline"
        "pod-security.kubernetes.io/audit"   = "baseline"
      }
    }
  })
}

resource "kubectl_manifest" "settings" {
  yaml_body = yamlencode({
    apiVersion = "v1"
    kind       = "ConfigMap"

    metadata = {
      namespace = local.namespace
      name      = "settings"
    }

    data = {
      "motd.txt" = <<-EOT
        key = { not = [1, 2] }
        Served from ${local.domain}.
      EOT
    }
  })

  depends_on = [
    kubectl_manifest.namespace
  ]
}

# One replica set, restarted when the settings change.
resource "kubectl_manifest" "web" {
  yaml_body = yamlencode({
    apiVersion = "apps/v1"
    kind       = "Deployment"

    metadata = {
      namespace = local.namespace
      name      = "web"
    }

    spec = {
      selector = { matchLabels = { app = "web" } }
      replicas = var.replicas

      template = {
        metadata = { labels = { app = "web" } }

        spec = {
          containers = [{
            image = "registry.example.test/web:1.2.3"
            ports = [{ containerPort = 8080 }]
            name  = "web"

            args = [
              "--listen",
              ":8080",
              "--upstream",
              local.ips.storage
            ]

            env = [
              {
                value = var.api_token
                name  = "API_TOKEN"
              },

              {
                value = join(",", local.internal_ranges)
                name  = "ALLOWED"
              }
            ]
          }]
        }
      }
    }
  })

  lifecycle {
    ignore_changes = [field_manager]
  }

  depends_on = [
    kubectl_manifest.namespace,
    kubectl_manifest.settings
  ]
}

resource "kubectl_manifest" "upstream" {
  yaml_body = each.value
  for_each  = data.kubectl_file_documents.upstream.manifests

  depends_on = [
    kubectl_manifest.namespace
  ]
}

output "hosts" {
  description = "Names served by the proxy."
  value       = local.hosts
}
