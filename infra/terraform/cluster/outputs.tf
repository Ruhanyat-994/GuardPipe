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

output "jwt_secret_arn" {
  value = data.terraform_remote_state.persistent.outputs.jwt_secret_arn
}

output "encryption_key_secret_arn" {
  value = data.terraform_remote_state.persistent.outputs.encryption_key_secret_arn
}

output "gemini_api_key_secret_arn" {
  value = data.terraform_remote_state.persistent.outputs.gemini_api_key_secret_arn
}

output "ecr_repository_urls" {
  value = data.terraform_remote_state.persistent.outputs.ecr_repository_urls
}

# The ALB's own hostname isn't an output here — it doesn't exist until the
# Ingress (deploy/k8s/07-ingress.yaml) is applied against this cluster, which
# happens after this apply, not as part of it (DEPLOYMENT.md §1a: infra.yml's
# "start" job prints it as its own workflow output once that step runs).
