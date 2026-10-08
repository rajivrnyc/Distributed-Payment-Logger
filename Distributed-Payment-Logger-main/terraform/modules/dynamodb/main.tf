# Idempotency table (TTL)
resource "aws_dynamodb_table" "idempotency" {
  name         = "${var.project_name}-Idempotency"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"

  attribute {
    name = "id"
    type = "S"
  }

  ttl {
    attribute_name = "ttl"
    enabled        = true
  }

  server_side_encryption {
    enabled = true
  }

  tags = { Name = "${var.project_name}-Idempotency" }
}

# Payments table
resource "aws_dynamodb_table" "payments" {
  name         = "${var.project_name}-Payments"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "pk"

  attribute {
    name = "pk"
    type = "S"
  }

  attribute {
    name = "accountId"
    type = "S"
  }


  global_secondary_index {
    name            = "GSI1"
    hash_key        = "accountId"
    projection_type = "ALL"
  }

  server_side_encryption { enabled = true }
  tags = { Name = "${var.project_name}-Payments" }
}

# Balances table
resource "aws_dynamodb_table" "balances" {
  name         = "${var.project_name}-Balances"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "pk"
  range_key    = "sk"

  attribute {
    name = "pk"
    type = "S"
  }

  attribute {
    name = "sk"
    type = "S"
  }

  server_side_encryption {
    enabled = true
  }

  tags = {
    Name = "${var.project_name}-Balances"
  }
}

output "table_names" {
  value = {
    idempotency = aws_dynamodb_table.idempotency.name
    payments    = aws_dynamodb_table.payments.name
    balances    = aws_dynamodb_table.balances.name
  }
}
