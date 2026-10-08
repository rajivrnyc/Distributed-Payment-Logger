output "alb_dns_name"            { value = module.ecs.alb_dns_name }
output "vpc_id"                  { value = module.network.vpc_id }
output "public_subnet_ids"       { value = module.network.public_subnet_ids }
output "private_subnet_ids"      { value = module.network.private_subnet_ids }
output "security_group_ecs"      { value = module.ecs.ecs_service_sg_id }
output "security_group_alb"      { value = module.ecs.alb_sg_id }
output "dynamodb_tables"         { value = module.dynamodb.table_names }
output "ecr_repos"               { value = {
  api       = aws_ecr_repository.api.repository_url
#  processor = aws_ecr_repository.processor.repository_url
#  replay    = aws_ecr_repository.replay.repository_url
} }
output "kinesis_stream_name" {
  value       = module.kinesis.stream_name
  description = "Name of the Kinesis stream for payment events"
}
output "kinesis_stream_arn" {
  value       = module.kinesis.stream_arn
  description = "ARN of the Kinesis stream"
}
output "kinesis_checkpoint_table" {
  value       = module.kinesis.checkpoint_table_name
  description = "DynamoDB table for Kinesis consumer checkpoints"
}