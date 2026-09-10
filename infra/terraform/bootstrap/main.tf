# One-time, run-by-hand: creates the S3 bucket + DynamoDB lock table that
# hold Terraform's own remote state for persistent/ and cluster/ (DEPLOYMENT.md
# §5). State for a cluster you might recreate from a different machine
# mid-project is exactly how state drifts silently if it's just a local file
# — this is standard practice, not gold-plating for its own sake.

# S3 bucket names are globally unique across all of AWS, not just this
# account — a random suffix avoids a name collision with someone else's
# bucket without needing to bake in an AWS account ID before one exists.
resource "random_id" "state_suffix" {
  byte_length = 4
}

resource "aws_s3_bucket" "terraform_state" {
  bucket = "${var.project}-tfstate-${random_id.state_suffix.hex}"

  # This bucket holds the only copy of Terraform's state for the whole
  # deployment — an accidental `terraform destroy` here would be far worse
  # than the resource it names, so it refuses destruction until this line is
  # deliberately removed by hand.
  lifecycle {
    prevent_destroy = true
  }

  tags = {
    Project = var.project
    Purpose = "terraform-remote-state"
  }
}

resource "aws_s3_bucket_versioning" "terraform_state" {
  bucket = aws_s3_bucket.terraform_state.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "terraform_state" {
  bucket = aws_s3_bucket.terraform_state.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "terraform_state" {
  bucket                  = aws_s3_bucket.terraform_state.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# Native S3 locking (`use_lockfile`, backend protocol v1.11+) is a real
# alternative that would drop the DynamoDB table entirely — not used here
# because it needs a Terraform/provider version newer than what's pinned
# above; a DynamoDB lock table costs pennies at this scale (PAY_PER_REQUEST,
# a handful of applies a month) so there's no real cost pressure to chase
# removing it.
resource "aws_dynamodb_table" "terraform_lock" {
  name         = "${var.project}-tfstate-lock"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "LockID"

  attribute {
    name = "LockID"
    type = "S"
  }

  lifecycle {
    prevent_destroy = true
  }

  tags = {
    Project = var.project
    Purpose = "terraform-remote-state-lock"
  }
}
