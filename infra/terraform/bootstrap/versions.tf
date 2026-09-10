# Bootstrap has no remote backend of its own — it's what CREATES the S3
# bucket/DynamoDB table that persistent/ and cluster/ use as their backend,
# so it can't depend on them existing yet. State for this config stays local
# (a single gitignored .tfstate on whoever's machine runs it) — acceptable
# because this config changes essentially never after the first apply.
#
# Every provider version pinned deliberately (CLAUDE.md applies the same
# reasoning to Gin/k8s.io — an unpinned provider silently changing must not
# be able to break `terraform apply` on a machine set up weeks apart).
terraform {
  required_version = ">= 1.9.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

provider "aws" {
  region = var.aws_region
}
