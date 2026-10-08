terraform {
  required_version = ">= 1.6.0"
  required_providers {
    aws    = { source = "hashicorp/aws",    version = ">= 5.0" }
    docker = { source = "kreuzwerker/docker", version = "~> 3.0" }
  }
}

provider "aws" {
  region = var.aws_region
}

# ECR auth token for Docker provider to push
data "aws_ecr_authorization_token" "registry" {}

provider "docker" {
  registry_auth {
    address  = data.aws_ecr_authorization_token.registry.proxy_endpoint
    username = data.aws_ecr_authorization_token.registry.user_name
    password = data.aws_ecr_authorization_token.registry.password
  }
}
