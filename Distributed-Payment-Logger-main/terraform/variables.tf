variable "project_name" {
  description = "Project prefix for resource naming."
  type        = string
  default     = "event-payments"
}

variable "aws_region" {
  description = "AWS region to deploy to."
  type        = string
  default     = "us-west-2"
}

# ---------- Networking ----------
variable "vpc_cidr" {
  description = "CIDR for the VPC."
  type        = string
  default     = "10.20.0.0/16"
}

variable "azs" {
  description = "Availability Zones to use."
  type        = list(string)
  default     = ["us-west-2a", "us-west-2b", "us-west-2c"]
}

variable "public_subnet_cidrs" {
  description = "CIDRs for public subnets (one per AZ)."
  type        = list(string)
  default     = ["10.20.0.0/24", "10.20.1.0/24", "10.20.2.0/24"]
}

variable "private_subnet_cidrs" {
  description = "CIDRs for private subnets (one per AZ)."
  type        = list(string)
  default     = ["10.20.10.0/24", "10.20.11.0/24", "10.20.12.0/24"]
}

variable "one_nat_gateway" {
  description = "Use a single NAT Gateway to reduce cost."
  type        = bool
  default     = true
}

variable "alb_ingress_cidr" {
  description = "CIDR allowed to reach the ALB."
  type        = string
  default     = "0.0.0.0/0"
}

# ---------- ECS sizing ----------
variable "api_cpu" {
  description = "Fargate CPU units (e.g., 256, 512, 1024)."
  type        = number
  default     = 256
}

variable "api_memory" {
  description = "Fargate memory in MB (e.g., 512, 1024, 2048)."
  type        = number
  default     = 512
}

variable "api_desired_count" {
  description = "ECS desired count for the API service."
  type        = number
  default     = 2
}

# ---------- Image tag ----------
variable "api_image_tag" {
  description = "Tag for the API image pushed to ECR."
  type        = string
  default     = "latest"
}

# ---------- Kafka ----------
variable "kafka_payments_topic" {
  description = "Kafka topic for payment events."
  type        = string
  default     = "payments.events"
}

variable "kafka_bootstrap_servers" {
  description = "Kafka bootstrap servers."
  type        = string
  default     = ""  # required when enable_msk = false
  nullable     = true
}

# ---------- Optional MSK ----------
variable "enable_msk" {
  description = "Whether to create an MSK Serverless cluster."
  type        = bool
  default     = false
}