variable "aws_region" {
  description = "AWS region everything in this deployment lives in."
  type        = string
  default     = "ap-northeast-1"
}

variable "project" {
  description = "Short name used as a prefix/tag for every resource this config creates."
  type        = string
  default     = "guardpipe"
}

# Fixed ahead of time (not generated) so persistent/'s subnet tags
# (kubernetes.io/cluster/<name>) are correct before the cluster itself
# exists — cluster/ must create its EKS cluster with this exact name.
variable "cluster_name" {
  description = "Name the EKS cluster (created later, in cluster/) will use. Fixed here so persistent/'s subnets can be pre-tagged for ALB/EKS discovery."
  type        = string
  default     = "guardpipe"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC."
  type        = string
  default     = "10.0.0.0/16"
}

# Two AZs because EKS's control plane ENIs must span at least two
# (DEPLOYMENT.md §3) and RDS's own DB subnet group requires the same — not a
# choice, a hard requirement of both services. Node group placement (later,
# in cluster/) still concentrates workers in one AZ to avoid cross-AZ data
# charges; only the subnets themselves need to exist in two.
variable "az_count" {
  description = "Number of availability zones to spread subnets across (minimum 2 — required by both EKS and RDS)."
  type        = number
  default     = 2
  validation {
    condition     = var.az_count >= 2
    error_message = "At least 2 AZs are required (EKS control plane + RDS subnet group both mandate it)."
  }
}

variable "db_name" {
  description = "Application database name inside the RDS instance."
  type        = string
  default     = "guardpipe"
}

variable "db_username" {
  description = "RDS master username. The master *password* is never set here — see manage_master_user_password in main.tf."
  type        = string
  default     = "guardpipe"
}

# Free-Tier-eligible as of the AWS Free Tier's current terms (750 hrs/mo of a
# small instance, 12 months from account creation — DEPLOYMENT.md §1a/§1b).
# Confirm this class is still listed as Free-Tier-eligible in the AWS
# Console before the first real apply; if not, the next size up is a small,
# known cost, not a surprise.
variable "db_instance_class" {
  description = "RDS instance class."
  type        = string
  default     = "db.t4g.micro"
}

variable "db_allocated_storage_gb" {
  description = "RDS allocated storage in GB (Free Tier covers up to 20GB)."
  type        = number
  default     = 20
}

variable "db_engine_version" {
  description = "PostgreSQL major.minor version — matches CLAUDE.md's \"PostgreSQL 16 is the system of record.\" 16.4 (this repo's original pin) has since been retired by AWS; confirmed 2026-09-12 via `aws rds describe-db-engine-versions` that 16.15 is current and orderable for db.t4g.micro in ap-northeast-1."
  type        = string
  default     = "16.15"
}

variable "ecr_repository_names" {
  description = "ECR repositories to create — one per image the existing Dockerfiles build."
  type        = list(string)
  default     = ["guardpipe", "guardpipe-web"]
}

variable "ecr_image_retention_count" {
  description = "How many tagged images to keep per ECR repo before the lifecycle policy expires the oldest (cost control at this scale)."
  type        = number
  default     = 10
}
