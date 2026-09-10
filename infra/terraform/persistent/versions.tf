# Applied once, essentially never destroyed (DEPLOYMENT.md §1a/§5): VPC,
# RDS, ECR, Secrets Manager, the "cluster access" SG. cluster/ reads this
# config's outputs via terraform_remote_state — never re-creates any of it.
#
# Backend is a *partial* S3 config on purpose: the bucket name bootstrap/
# creates includes a random suffix that doesn't exist until bootstrap has
# been applied, and Terraform backend blocks can't reference variables or
# other resources' outputs. Fill in the real values (from bootstrap/'s
# outputs) in a gitignored backend.hcl, copied from backend.hcl.example, then
# run `terraform init -backend-config=backend.hcl`.
terraform {
  required_version = ">= 1.9.0"

  backend "s3" {
    # bucket / key / region / dynamodb_table supplied via -backend-config=backend.hcl
    encrypt = true
  }

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project = var.project
      Layer   = "persistent"
      Managed = "terraform"
    }
  }
}
