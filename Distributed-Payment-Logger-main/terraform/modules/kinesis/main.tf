resource "aws_kinesis_stream" "payment_events" {
  name             = "event-payments-${var.environment}"
  retention_period = var.retention_period

  shard_level_metrics = [
    "IncomingBytes",
    "IncomingRecords",
    "OutgoingBytes",
    "OutgoingRecords",
  ]

  stream_mode_details {
    stream_mode = "PROVISIONED"
  }

  shard_count = var.shard_count

  tags = merge(
    var.tags,
    {
      Name = "event-payments-${var.environment}"
    }
  )
}

# DynamoDB table for consumer checkpointing
resource "aws_dynamodb_table" "kinesis_checkpoints" {
  name           = "event-payments-kinesis-checkpoints-${var.environment}"
  billing_mode   = "PAY_PER_REQUEST"
  hash_key       = "ConsumerGroup"
  range_key      = "ShardId"

  attribute {
    name = "ConsumerGroup"
    type = "S"
  }

  attribute {
    name = "ShardId"
    type = "S"
  }

  tags = merge(
    var.tags,
    {
      Name = "event-payments-kinesis-checkpoints-${var.environment}"
    }
  )
}
