# The ephemeral layer (DEPLOYMENT.md §1a/§5): EKS control plane, node group,
# IRSA roles, ALB controller IAM, budgets alert. Created on "start" (infra.yml
# apply), destroyed on "stop" (infra.yml destroy) — never touches persistent/.
#
# Backend is a partial S3 config, same reasoning as persistent/versions.tf:
# fill in backend.hcl (gitignored, copied from backend.hcl.example) with
# bootstrap/'s outputs, then `terraform init -backend-config=backend.hcl`.
terraform {
  required_version = ">= 1.9.0"

  backend "s3" {
    encrypt = true
  }

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project = var.project
      Layer   = "cluster"
      Managed = "terraform"
    }
  }
}

# persistent/'s own state — read-only, via terraform_remote_state (DEPLOYMENT.md
# §5: "cluster/ reading persistent/'s outputs via a terraform_remote_state
# data source" — no data lookups needed in the other direction).
data "terraform_remote_state" "persistent" {
  backend = "s3"
  config = {
    bucket         = var.state_bucket
    key            = "persistent/terraform.tfstate"
    region         = var.state_region
    dynamodb_table = var.state_dynamodb_table
  }
}
