variable "environment" {
  description = "Environment name"
  type        = string
}

variable "retention_period" {
  description = "Kinesis stream retention period in hours (24-8760)"
  type        = number
  default     = 168 # 7 days
}

variable "shard_count" {
  description = "Number of shards for the stream"
  type        = number
  default     = 2
}

variable "tags" {
  description = "Tags to apply to resources"
  type        = map(string)
  default     = {}
}
