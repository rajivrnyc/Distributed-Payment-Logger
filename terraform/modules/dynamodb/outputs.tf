output "idempotency_table_name" { value = aws_dynamodb_table.idempotency.name }
output "payments_table_name"    { value = aws_dynamodb_table.payments.name }
output "balances_table_name"    { value = aws_dynamodb_table.balances.name }