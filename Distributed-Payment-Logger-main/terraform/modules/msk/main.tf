# Optional MSK Serverless. When disabled, output empty details.
locals {
  enabled = var.enable_msk
}

resource "aws_msk_serverless_cluster" "this" {
  count = local.enabled ? 1 : 0

  cluster_name = var.msk_cluster_name

  vpc_config {
    subnet_ids         = var.private_subnet_ids
    security_group_ids = var.security_group_ids
  }

  client_authentication {
    sasl {
      iam {
        enabled = true
      }
    }
  }

  tags = { Name = var.msk_cluster_name }
}

# Outputs
output "bootstrap_brokers" {
  value = local.enabled ? aws_msk_serverless_cluster.this[0].bootstrap_brokers_sasl_iam : ""
}

output "cluster_arn" {
  value = local.enabled ? aws_msk_serverless_cluster.this[0].arn : ""
}
