output "vpc_id" {
  description = "VPC ID — cluster/ places the EKS control plane and node group here."
  value       = aws_vpc.main.id
}

output "public_subnet_ids" {
  description = "All public subnet IDs (one per AZ) — EKS control plane spans all of these; node group placement (cluster/) picks a subset to concentrate workers in one AZ."
  value       = aws_subnet.public[*].id
}

output "cluster_access_security_group_id" {
  description = "Security group the EKS node group (cluster/) must join as an extra SG so RDS's ingress rule (which allows this group) actually applies to it."
  value       = aws_security_group.cluster_access.id
}

output "rds_endpoint" {
  description = "RDS endpoint hostname (no port) — combine with rds_port and the managed master-password secret to assemble GUARDPIPE_DATABASE_URL at deploy time."
  value       = aws_db_instance.main.address
}

output "rds_port" {
  value = aws_db_instance.main.port
}

output "rds_db_name" {
  value = aws_db_instance.main.db_name
}

output "rds_master_user_secret_arn" {
  description = "ARN of the RDS-managed Secrets Manager secret holding the actual master password — never a Terraform-managed value."
  value       = aws_db_instance.main.master_user_secret[0].secret_arn
}

output "ecr_repository_urls" {
  description = "Map of repo name -> full ECR repository URL, for deploy.yml's docker push/tag step."
  value       = { for name, repo in aws_ecr_repository.images : name => repo.repository_url }
}

## SSM Parameter Store: the CSI driver's SecretProviderClass wants the
## parameter *name*, not an ARN (unlike Secrets Manager, whose objectName
## also accepts a friendly name — the ssmparameter objectType only takes
## the name) — the _arn outputs exist purely for cluster/'s IAM policy
## resources, never used for the CSI substitution.

output "jwt_secret_parameter_name" {
  value = aws_ssm_parameter.jwt_secret.name
}

output "jwt_secret_parameter_arn" {
  value = aws_ssm_parameter.jwt_secret.arn
}

output "encryption_key_parameter_name" {
  value = aws_ssm_parameter.encryption_key.name
}

output "encryption_key_parameter_arn" {
  value = aws_ssm_parameter.encryption_key.arn
}

output "gemini_api_key_parameter_name" {
  value = aws_ssm_parameter.gemini_api_key.name
}

output "gemini_api_key_parameter_arn" {
  value = aws_ssm_parameter.gemini_api_key.arn
}

output "sonarqube_token_parameter_name" {
  value = aws_ssm_parameter.sonarqube_token.name
}

output "sonarqube_token_parameter_arn" {
  value = aws_ssm_parameter.sonarqube_token.arn
}

output "ses_identity_arn" {
  description = "ARN of the SES sender identity, or \"\" when none is configured. cluster/ scopes the app role's ses:SendEmail permission to it."
  value       = length(aws_sesv2_email_identity.sender) > 0 ? aws_sesv2_email_identity.sender[0].arn : ""
}

output "ses_dkim_records" {
  description = "DKIM CNAME records to add to the sender domain's DNS (domain identities only)."
  value = length(aws_sesv2_email_identity.sender) > 0 ? [
    for t in try(aws_sesv2_email_identity.sender[0].dkim_signing_attributes[0].tokens, []) : {
      name  = "${t}._domainkey.${var.ses_sender_identity}"
      type  = "CNAME"
      value = "${t}.dkim.amazonses.com"
    }
  ] : []
}
