variable "aws_region" {
  description = "AWS region — must match persistent/'s region."
  type        = string
  default     = "ap-northeast-1"
}

variable "project" {
  description = "Short name used as a prefix/tag for every resource this config creates."
  type        = string
  default     = "guardpipe"
}

variable "cluster_name" {
  description = "EKS cluster name — must exactly match persistent/'s var.cluster_name (its subnets are pre-tagged for this name's ALB/EKS discovery)."
  type        = string
  default     = "guardpipe"
}

# Remote-state lookup of persistent/'s outputs (DEPLOYMENT.md §5) — these
# three identify *where* that state lives, not what's in it.
variable "state_bucket" {
  description = "S3 bucket name from bootstrap/'s state_bucket_name output."
  type        = string
}

variable "state_dynamodb_table" {
  description = "DynamoDB table name from bootstrap/'s state_lock_table_name output."
  type        = string
}

variable "state_region" {
  description = "Region the Terraform state bucket/lock table live in."
  type        = string
  default     = "ap-northeast-1"
}

# Confirm this is still a currently-supported EKS version in the AWS Console
# before the first real apply (AWS deprecates old versions on its own
# schedule) — same "verify before relying on it" caution as persistent/'s
# db_instance_class Free-Tier note. 1.31 (this repo's original pin) left
# standard support on 2025-11-26 — checked 2026-09-12 via
# `aws eks describe-cluster-versions`, EKS would silently bill Extended
# Support surcharges on top of the control-plane hourly rate for it now. 1.32
# is the oldest version still inside standard support (until 2026-03-23).
variable "cluster_version" {
  description = "Kubernetes version for the EKS control plane."
  type        = string
  default     = "1.32"
}

# Fixed node count, no autoscaler (DEPLOYMENT.md §3: "at this scale,
# autoscaling infrastructure costs more in complexity than it saves in
# compute"). min = max = desired disables any scaling *action*; nothing
# outside a manual `terraform apply` changes this number.
variable "node_desired_size" {
  # 2 -> 3 (2026-09-14): SonarQube (deploy/k8s/addons/sonarqube/) needs real
  # memory (it runs Elasticsearch internally, realistically 3-4GB) — close
  # to a whole t3.medium on its own. One more node gives it room without
  # starving the app's own pods on the original two.
  type    = number
  default = 3
}

variable "node_min_size" {
  type    = number
  default = 3
}

variable "node_max_size" {
  type    = number
  default = 3
}

variable "node_instance_types" {
  description = "EC2 instance types for the managed node group."
  type        = list(string)
  default     = ["t3.medium"]
}

variable "node_capacity_type" {
  description = "SPOT or ON_DEMAND."
  type        = string
  default     = "SPOT"
}

# The AWS Load Balancer Controller's own convention (its raw-manifest release
# hardcodes this) — not this project's choice to make, so not worth a
# variable for the value itself, only documented here for where it's used.
variable "alb_controller_namespace" {
  type    = string
  default = "kube-system"
}

variable "alb_controller_service_account" {
  type    = string
  default = "aws-load-balancer-controller"
}

# Must match deploy/k8s/00-namespace.yaml and the ServiceAccount names the
# api/worker Deployments use (Part 5) — this is where the IRSA trust policy
# and the k8s-manifest side of the same contract have to agree.
variable "app_namespace" {
  type    = string
  default = "guardpipe"
}

variable "app_service_accounts" {
  description = "Service account names (in app_namespace) that need read access to the app secrets (Secrets Manager for the RDS password, SSM Parameter Store for the rest) — one shared IRSA role covers both, least privilege scoped to the exact resources in main.tf."
  type        = list(string)
  default     = ["guardpipe-api", "guardpipe-worker"]
}

variable "budget_limit_usd" {
  description = "AWS Budgets monthly alert threshold (DEPLOYMENT.md §3)."
  type        = number
  default     = 25
}

variable "github_repo" {
  description = "GitHub \"owner/repo\" this cluster's CI role trusts (github-oidc.tf) — must match exactly, it's the whole security boundary for who can assume the role."
  type        = string
  default     = "Ruhanyat-994/GuardPipe"
}

variable "budget_alert_email" {
  description = "Email address the $25 AWS Budgets alert notifies."
  type        = string
  validation {
    condition     = length(trimspace(var.budget_alert_email)) > 0
    error_message = "budget_alert_email is required — this is the one guardrail against a silent runaway bill."
  }
}
