output "state_bucket_name" {
  description = "S3 bucket name — persistent/ and cluster/ point their backend config at this."
  value       = aws_s3_bucket.terraform_state.id
}

output "state_lock_table_name" {
  description = "DynamoDB table name — persistent/ and cluster/ point their backend config at this."
  value       = aws_dynamodb_table.terraform_lock.name
}

output "aws_region" {
  description = "Region the state bucket/lock table live in — persistent/ and cluster/ backend config must match."
  value       = var.aws_region
}
