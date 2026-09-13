output "cluster_name" {
  value = aws_eks_cluster.main.name
}

output "cluster_endpoint" {
  value = aws_eks_cluster.main.endpoint
}

output "cluster_certificate_authority_data" {
  value = aws_eks_cluster.main.certificate_authority[0].data
}

output "alb_controller_role_arn" {
  description = "Annotate the aws-load-balancer-controller ServiceAccount (deploy/k8s/addons/) with eks.amazonaws.com/role-arn = this."
  value       = aws_iam_role.alb_controller.arn
}

output "guardpipe_app_role_arn" {
  description = "Annotate the guardpipe-api and guardpipe-worker ServiceAccounts (deploy/k8s/04-api.yaml, 05-worker.yaml) with eks.amazonaws.com/role-arn = this."
  value       = aws_iam_role.guardpipe_app.arn
}

output "oidc_provider_arn" {
  value = aws_iam_openid_connect_provider.eks.arn
}

output "github_actions_role_arn" {
  description = "Set this as the GitHub repo Variable AWS_OIDC_ROLE_ARN (DEPLOYMENT-NEXT-STEPS.md Part 9) — deploy.yml/infra.yml assume this role via OIDC, no static AWS keys."
  value       = aws_iam_role.github_actions.arn
}

## ---------------------------------------------------------------------------
## Pass-throughs of persistent/'s own outputs — so infra.yml's "start" job
## can get everything deploy/k8s/'s manifests need to be substituted
## (envsubst) from a single `terraform output -json` here, without a second
## `terraform init`/state lookup against persistent/'s own backend config.
## ---------------------------------------------------------------------------

output "rds_endpoint" {
  value = data.terraform_remote_state.persistent.outputs.rds_endpoint
}

output "rds_port" {
  value = data.terraform_remote_state.persistent.outputs.rds_port
}

output "rds_db_name" {
  value = data.terraform_remote_state.persistent.outputs.rds_db_name
}

output "rds_master_user_secret_arn" {
  value = data.terraform_remote_state.persistent.outputs.rds_master_user_secret_arn
}

output "jwt_secret_parameter_name" {
  value = data.terraform_remote_state.persistent.outputs.jwt_secret_parameter_name
}

output "encryption_key_parameter_name" {
  value = data.terraform_remote_state.persistent.outputs.encryption_key_parameter_name
}

output "gemini_api_key_parameter_name" {
  value = data.terraform_remote_state.persistent.outputs.gemini_api_key_parameter_name
}

output "ecr_repository_urls" {
  value = data.terraform_remote_state.persistent.outputs.ecr_repository_urls
}

output "vpc_id" {
  # Fed to the ALB controller as --aws-vpc-id (deploy/k8s/addons/aws-load-
  # balancer-controller-v3.5.0-full.yaml) so it never needs to discover the
  # VPC via EC2 instance metadata — that lookup fails from a pod's network
  # namespace under the account's default IMDS hop limit of 1 (an extra
  # network hop pod traffic takes versus host-network traffic), confirmed
  # live 2026-09-13: "failed to fetch VPC ID from instance metadata ...
  # context deadline exceeded", crash-looping the controller. Passing the
  # VPC ID explicitly sidesteps IMDS entirely rather than reconfiguring the
  # node launch template's metadata_options, which would need replacing
  # already-running node instances.
  value = data.terraform_remote_state.persistent.outputs.vpc_id
}

# The ALB's own hostname isn't an output here — it doesn't exist until the
# Ingress (deploy/k8s/07-ingress.yaml) is applied against this cluster, which
# happens after this apply, not as part of it (DEPLOYMENT.md §1a: infra.yml's
# "start" job prints it as its own workflow output once that step runs).
