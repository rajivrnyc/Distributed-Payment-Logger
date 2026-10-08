variable "aws_region" { type = string }
variable "ddb_table_idempotency" { type = string }
variable "ddb_table_payments" { type = string }
variable "ddb_table_balances" { type = string }
variable "kafka_bootstrap_servers" { type = string }
variable "kafka_topic" { type = string }
variable "project_name" { type = string }
variable "vpc_id" { type = string }
variable "public_subnet_ids" { type = list(string) }
variable "private_subnet_ids" { type = list(string) }
variable "alb_ingress_cidr" { type = string }
variable "create_alb" {
  type    = bool
  default = true
}
variable "ecs_task_execution_role_arn" {
  type        = string
  description = "ARN of the ECS task execution role"
}
variable "ecs_task_role_arn" {
  type        = string
  description = "ARN of the ECS task role"
}
variable "processor_desired_count" {
  type        = number
  description = "Desired count for processor service"
  default     = 0
}
variable "kinesis_stream_name" {
  type        = string
  description = "Name of the Kinesis stream"
  default     = ""
}
variable "kinesis_checkpoint_table" {
  type        = string
  description = "Name of the DynamoDB checkpoint table for Kinesis"
  default     = ""
}
