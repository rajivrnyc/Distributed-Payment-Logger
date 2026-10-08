output "stream_name" {
  description = "Name of the Kinesis stream"
  value       = aws_kinesis_stream.payment_events.name
}

output "stream_arn" {
  description = "ARN of the Kinesis stream"
  value       = aws_kinesis_stream.payment_events.arn
}

output "checkpoint_table_name" {
  description = "Name of the DynamoDB checkpoint table"
  value       = aws_dynamodb_table.kinesis_checkpoints.name
}

output "checkpoint_table_arn" {
  description = "ARN of the DynamoDB checkpoint table"
  value       = aws_dynamodb_table.kinesis_checkpoints.arn
}
