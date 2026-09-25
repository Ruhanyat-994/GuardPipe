## ---------------------------------------------------------------------------
## Data sources
## ---------------------------------------------------------------------------

data "aws_availability_zones" "available" {
  state = "available"
}

## ---------------------------------------------------------------------------
## VPC + subnets
##
## Public subnets only, no NAT Gateway (DEPLOYMENT.md §3: a managed NAT
## Gateway alone is ~$32/mo, more than the entire budget). Worker nodes
## (created later, in cluster/) get public IPs and sit behind a tight
## security group instead — a reasonable trade for a short-lived personal
## demo cluster with no other tenants, not the choice for a real multi-tenant
## production system.
## ---------------------------------------------------------------------------

resource "aws_vpc" "main" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = {
    Name = "${var.project}-vpc"
  }
}

resource "aws_internet_gateway" "main" {
  vpc_id = aws_vpc.main.id

  tags = {
    Name = "${var.project}-igw"
  }
}

resource "aws_subnet" "public" {
  count = var.az_count

  vpc_id                  = aws_vpc.main.id
  cidr_block              = cidrsubnet(var.vpc_cidr, 4, count.index)
  availability_zone       = data.aws_availability_zones.available.names[count.index]
  map_public_ip_on_launch = true

  tags = {
    Name = "${var.project}-public-${count.index}"
    # EKS/ALB discovery tags (DEPLOYMENT.md §3) — cluster/ doesn't exist yet
    # when these subnets are created, so the cluster name is fixed via
    # var.cluster_name rather than read from a resource that isn't there.
    "kubernetes.io/cluster/${var.cluster_name}" = "shared"
    "kubernetes.io/role/elb"                    = "1"
  }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.main.id

  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.main.id
  }

  tags = {
    Name = "${var.project}-public-rt"
  }
}

resource "aws_route_table_association" "public" {
  count = var.az_count

  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

## ---------------------------------------------------------------------------
## Security groups
##
## "cluster_access" is intentionally empty of rules here — it's a bootstrap
## seam (DEPLOYMENT.md §1a "the one bootstrapping wrinkle"): RDS's inbound
## rule allows *this* group, and the node group (created later, in cluster/,
## which doesn't exist yet) simply joins it as an extra security group every
## time it's created. No cross-layer remote-state lookup of a not-yet-created
## node group SG is needed for this to work.
## ---------------------------------------------------------------------------

resource "aws_security_group" "cluster_access" {
  name        = "${var.project}-cluster-access"
  description = "Joined by the EKS node group (cluster/) so RDS can allow it by SG reference without a circular dependency."
  vpc_id      = aws_vpc.main.id

  tags = {
    Name = "${var.project}-cluster-access"
  }
}

resource "aws_security_group" "rds" {
  name        = "${var.project}-rds"
  description = "Allows Postgres only from the cluster-access SG that the node group joins."
  vpc_id      = aws_vpc.main.id

  ingress {
    description     = "Postgres from the EKS node group"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.cluster_access.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${var.project}-rds-sg"
  }
}

## ---------------------------------------------------------------------------
## RDS PostgreSQL — persistent, always-on, decoupled from the cluster's
## lifecycle (DEPLOYMENT.md §1a). Single-AZ (no standby) to stay inside the
## Free Tier and this deployment's actual availability needs.
## ---------------------------------------------------------------------------

resource "aws_db_subnet_group" "main" {
  name       = "${var.project}-db-subnet-group"
  subnet_ids = aws_subnet.public[*].id

  tags = {
    Name = "${var.project}-db-subnet-group"
  }
}

resource "aws_db_instance" "main" {
  identifier     = "${var.project}-db"
  engine         = "postgres"
  engine_version = var.db_engine_version

  instance_class    = var.db_instance_class
  allocated_storage = var.db_allocated_storage_gb
  storage_type      = "gp3"
  storage_encrypted = true

  db_name  = var.db_name
  username = var.db_username
  # RDS generates and stores the actual master password itself in Secrets
  # Manager, natively rotatable — it never passes through this config or
  # Terraform's own state as a literal value (DEPLOYMENT.md §6).
  manage_master_user_password = true

  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  publicly_accessible    = false
  multi_az               = false

  backup_retention_period = 7
  # A demo/resume deployment, not a production system holding data anyone
  # else depends on — a final snapshot on every accidental `terraform apply`
  # drift/replace would be one more manual step for no real benefit here.
  # Flip to false and set deletion_protection = true first if this ever
  # holds data worth protecting from a fat-fingered destroy.
  skip_final_snapshot = true
  deletion_protection = false

  tags = {
    Name = "${var.project}-db"
  }
}

## ---------------------------------------------------------------------------
## ECR — one repo per image the existing Dockerfiles already build
## (DEPLOYMENT.md §3/§4).
## ---------------------------------------------------------------------------

resource "aws_ecr_repository" "images" {
  for_each = toset(var.ecr_repository_names)

  name                 = each.value
  image_tag_mutability = "IMMUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  tags = {
    Name = each.value
  }
}

resource "aws_ecr_lifecycle_policy" "images" {
  for_each   = aws_ecr_repository.images
  repository = each.value.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Expire untagged images after 3 days"
        selection = {
          tagStatus   = "untagged"
          countType   = "sinceImagePushed"
          countUnit   = "days"
          countNumber = 3
        }
        action = { type = "expire" }
      },
      {
        rulePriority = 2
        description  = "Keep only the most recent ${var.ecr_image_retention_count} tagged images"
        selection = {
          tagStatus     = "tagged"
          tagPrefixList = ["sha-"]
          countType     = "imageCountMoreThan"
          countNumber   = var.ecr_image_retention_count
        }
        action = { type = "expire" }
      }
    ]
  })
}

## ---------------------------------------------------------------------------
## SSM Parameter Store (Standard tier, SecureString) — application secrets.
## Cheaper than Secrets Manager for these three: Standard-tier parameters
## are $0/month storage and $0 per API call (vs. $0.40/secret/month + $0.05
## per 10k calls each), and none of these three ever needed Secrets
## Manager's native rotation — they're populated by hand once, same as
## before. Encrypted with the AWS-managed `alias/aws/ssm` KMS key (free);
## a customer-managed key would add a flat $1/month regardless of which
## secret store it backs, so deliberately not used here.
##
## The RDS master password stays in Secrets Manager (below, via
## manage_master_user_password) — that one can't move, since RDS's native
## rotation is what actually manages it.
##
## Terraform's SSM parameter resource requires a `value` at creation time
## (unlike Secrets Manager, which allows a container with no version at
## all) — so `value` starts as an obvious placeholder and
## `lifecycle.ignore_changes` stops every later `apply` from stomping the
## real value back to it once it's set by hand
## (`aws ssm put-parameter --overwrite`), never written into this config or
## Terraform state.
## ---------------------------------------------------------------------------

resource "aws_ssm_parameter" "jwt_secret" {
  name        = "/${var.project}/jwt-secret"
  description = "GUARDPIPE_JWT_SECRET — value populated by hand after apply, never by Terraform."
  type        = "SecureString"
  tier        = "Standard"
  value       = "REPLACE_ME_MANUALLY_AFTER_APPLY"

  lifecycle {
    ignore_changes = [value]
  }
}

resource "aws_ssm_parameter" "encryption_key" {
  name        = "/${var.project}/encryption-key"
  description = "GUARDPIPE_ENCRYPTION_KEY — value populated by hand after apply, never by Terraform."
  type        = "SecureString"
  tier        = "Standard"
  value       = "REPLACE_ME_MANUALLY_AFTER_APPLY"

  lifecycle {
    ignore_changes = [value]
  }
}

resource "aws_ssm_parameter" "gemini_api_key" {
  name        = "/${var.project}/gemini-api-key"
  description = "GUARDPIPE_GEMINI_API_KEY(S) — value populated by hand after apply, never by Terraform."
  type        = "SecureString"
  tier        = "Standard"
  value       = "REPLACE_ME_MANUALLY_AFTER_APPLY"

  lifecycle {
    ignore_changes = [value]
  }
}

resource "aws_ssm_parameter" "sonarqube_token" {
  # GUARDPIPE_SONARQUBE_TOKEN — codescan (ADR-0011)'s own API credential,
  # generated once in SonarQube's own UI on first boot
  # (deploy/k8s/addons/sonarqube/), not by Terraform. Unlike SonarQube's own
  # DB password (a plain Kubernetes Secret in the sonarqube namespace, never
  # read by the app), this one IS read by guardpipe-api/-worker, so it goes
  # through the same SSM + Secrets Store CSI driver pipeline as
  # jwt-secret/encryption-key/gemini-api-key above.
  name        = "/${var.project}/sonarqube-token"
  description = "GUARDPIPE_SONARQUBE_TOKEN — value populated by hand after apply, never by Terraform."
  type        = "SecureString"
  tier        = "Standard"
  value       = "REPLACE_ME_MANUALLY_AFTER_APPLY"

  lifecycle {
    ignore_changes = [value]
  }
}

## ---------------------------------------------------------------------------
## Amazon SES — sender identity for scan-report emails (modules/notification).
## Persistent, not cluster/: verifying an identity (DNS records, or clicking
## AWS's email link) is a one-time manual step that must survive the cluster
## being torn down and rebuilt. Nothing is created unless ses_sender_identity
## is set.
##
## Manual steps this config can't do for you (see DEPLOYMENT.md):
##   * domain identity: add the three DKIM CNAMEs from the ses_dkim_records
##     output to the domain's DNS;
##   * address identity: click the link AWS emails to that address;
##   * new SES accounts start in the sandbox (can only send TO verified
##     addresses, 200/day) — request production access in the SES console.
## ---------------------------------------------------------------------------
resource "aws_sesv2_email_identity" "sender" {
  count          = var.ses_sender_identity == "" ? 0 : 1
  email_identity = var.ses_sender_identity
}
