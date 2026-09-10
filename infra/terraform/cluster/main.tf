## ---------------------------------------------------------------------------
## EKS cluster IAM role
## ---------------------------------------------------------------------------

data "aws_iam_policy_document" "eks_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["eks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "eks_cluster" {
  name               = "${var.project}-eks-cluster"
  assume_role_policy = data.aws_iam_policy_document.eks_assume_role.json
}

resource "aws_iam_role_policy_attachment" "eks_cluster_policy" {
  role       = aws_iam_role.eks_cluster.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
}

## ---------------------------------------------------------------------------
## EKS control plane
##
## Placed in the *persistent* VPC's subnets (data.terraform_remote_state) —
## this config never creates its own network, only ever reuses persistent/'s
## (DEPLOYMENT.md §1a). Spans all public subnets (>=2 AZs, EKS's own
## control-plane ENI requirement); the node group below concentrates workers
## in a single one of them.
## ---------------------------------------------------------------------------

resource "aws_eks_cluster" "main" {
  name     = var.cluster_name
  role_arn = aws_iam_role.eks_cluster.arn
  version  = var.cluster_version

  vpc_config {
    subnet_ids              = data.terraform_remote_state.persistent.outputs.public_subnet_ids
    endpoint_public_access  = true
    endpoint_private_access = false
  }

  depends_on = [aws_iam_role_policy_attachment.eks_cluster_policy]

  tags = {
    Name = var.cluster_name
  }
}

## ---------------------------------------------------------------------------
## EKS-managed add-ons.
##
## vpc-cni: EKS auto-creates this (and coredns/kube-proxy) with default
## settings the moment the cluster exists, whether or not it's managed here
## — but the default has NetworkPolicy enforcement OFF. deploy/k8s/
## 08-networkpolicy.yaml's default-deny-plus-allows only actually does
## anything with it turned on (confirmed against the addon's own Helm chart
## source, aws/amazon-vpc-cni-k8s charts/aws-vpc-cni/values.yaml:
## `enableNetworkPolicy`) — without this, those NetworkPolicy objects would
## apply cleanly and enforce nothing, silently. resolve_conflicts_on_create
## = OVERWRITE because the auto-created default already exists by the time
## this resource is created.
## ---------------------------------------------------------------------------

resource "aws_eks_addon" "vpc_cni" {
  cluster_name = aws_eks_cluster.main.name
  addon_name   = "vpc-cni"
  configuration_values = jsonencode({
    enableNetworkPolicy = "true"
  })
  resolve_conflicts_on_create = "OVERWRITE"
  resolve_conflicts_on_update = "OVERWRITE"
}

resource "aws_eks_addon" "kube_proxy" {
  cluster_name                = aws_eks_cluster.main.name
  addon_name                  = "kube-proxy"
  resolve_conflicts_on_create = "OVERWRITE"
  resolve_conflicts_on_update = "OVERWRITE"
}

resource "aws_eks_addon" "coredns" {
  cluster_name                = aws_eks_cluster.main.name
  addon_name                  = "coredns"
  resolve_conflicts_on_create = "OVERWRITE"
  resolve_conflicts_on_update = "OVERWRITE"
  # CoreDNS pods need a node to actually schedule onto — unlike vpc-cni/
  # kube-proxy (node-local DaemonSets that come up as each node joins),
  # letting this wait on the node group avoids it sitting Pending during a
  # fresh "start".
  depends_on = [aws_eks_node_group.main]
}

## ---------------------------------------------------------------------------
## EKS managed node group — one group, fixed size, Spot (DEPLOYMENT.md §3).
##
## A launch template is the only way to attach an *extra* security group to
## a managed node group's instances — and specifying network_interfaces
## there means EKS no longer auto-attaches the cluster's own primary security
## group, so both it and persistent/'s cluster_access SG (the bootstrapping
## seam that lets RDS's ingress rule reference a group that predates the node
## group itself) are listed explicitly below.
## ---------------------------------------------------------------------------

data "aws_iam_policy_document" "ec2_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "eks_nodes" {
  name               = "${var.project}-eks-nodes"
  assume_role_policy = data.aws_iam_policy_document.ec2_assume_role.json
}

resource "aws_iam_role_policy_attachment" "eks_nodes_worker" {
  role       = aws_iam_role.eks_nodes.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
}

resource "aws_iam_role_policy_attachment" "eks_nodes_cni" {
  role       = aws_iam_role.eks_nodes.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
}

resource "aws_iam_role_policy_attachment" "eks_nodes_ecr" {
  role       = aws_iam_role.eks_nodes.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

resource "aws_launch_template" "nodes" {
  name_prefix            = "${var.project}-node-"
  update_default_version = true

  network_interfaces {
    security_groups = [
      aws_eks_cluster.main.vpc_config[0].cluster_security_group_id,
      data.terraform_remote_state.persistent.outputs.cluster_access_security_group_id,
    ]
  }

  tag_specifications {
    resource_type = "instance"
    tags = {
      Name = "${var.project}-node"
    }
  }
}

resource "aws_eks_node_group" "main" {
  cluster_name    = aws_eks_cluster.main.name
  node_group_name = "${var.project}-nodes"
  node_role_arn   = aws_iam_role.eks_nodes.arn

  # Only the first AZ's subnet (DEPLOYMENT.md §3: "worker nodes ... concentrated
  # in one AZ to avoid cross-AZ data charges" — the control plane above still
  # spans all of them, only workers are concentrated).
  subnet_ids = [data.terraform_remote_state.persistent.outputs.public_subnet_ids[0]]

  capacity_type  = var.node_capacity_type
  instance_types = var.node_instance_types
  ami_type       = "AL2023_x86_64_STANDARD"

  scaling_config {
    desired_size = var.node_desired_size
    min_size     = var.node_min_size
    max_size     = var.node_max_size
  }

  update_config {
    max_unavailable = 1
  }

  launch_template {
    id      = aws_launch_template.nodes.id
    version = aws_launch_template.nodes.latest_version
  }

  depends_on = [
    aws_iam_role_policy_attachment.eks_nodes_worker,
    aws_iam_role_policy_attachment.eks_nodes_cni,
    aws_iam_role_policy_attachment.eks_nodes_ecr,
  ]

  tags = {
    Name = "${var.project}-nodes"
  }
}

## ---------------------------------------------------------------------------
## IRSA — IAM OIDC provider for the cluster (DEPLOYMENT.md §3: "IRSA ...
## pods get exactly the AWS permissions they need, nothing broader").
## ---------------------------------------------------------------------------

data "tls_certificate" "eks_oidc" {
  url = aws_eks_cluster.main.identity[0].oidc[0].issuer
}

resource "aws_iam_openid_connect_provider" "eks" {
  url             = aws_eks_cluster.main.identity[0].oidc[0].issuer
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [data.tls_certificate.eks_oidc.certificates[0].sha1_fingerprint]
}

locals {
  oidc_provider_url_no_scheme = replace(aws_iam_openid_connect_provider.eks.url, "https://", "")
}

## ---------------------------------------------------------------------------
## IRSA — AWS Load Balancer Controller
##
## Policy JSON vendored verbatim from
## kubernetes-sigs/aws-load-balancer-controller's own release
## (docs/install/iam_policy.json @ tag v3.5.0, fetched 2026-09-11) — never
## hand-transcribed, so it can't silently drift from what that controller
## version actually needs. If the vendored controller manifest (deploy/k8s/
## addons/, Part 5) is ever bumped to a newer release, re-fetch this file
## from that same tag.
## ---------------------------------------------------------------------------

resource "aws_iam_policy" "alb_controller" {
  name   = "${var.project}-alb-controller"
  policy = file("${path.module}/policies/aws-load-balancer-controller-iam-policy.json")
}

data "aws_iam_policy_document" "alb_controller_assume_role" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.eks.arn]
    }
    condition {
      test     = "StringEquals"
      variable = "${local.oidc_provider_url_no_scheme}:sub"
      values   = ["system:serviceaccount:${var.alb_controller_namespace}:${var.alb_controller_service_account}"]
    }
    condition {
      test     = "StringEquals"
      variable = "${local.oidc_provider_url_no_scheme}:aud"
      values   = ["sts.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "alb_controller" {
  name               = "${var.project}-alb-controller"
  assume_role_policy = data.aws_iam_policy_document.alb_controller_assume_role.json
}

resource "aws_iam_role_policy_attachment" "alb_controller" {
  role       = aws_iam_role.alb_controller.name
  policy_arn = aws_iam_policy.alb_controller.arn
}

## ---------------------------------------------------------------------------
## IRSA — guardpipe-api / guardpipe-worker: least-privilege Secrets Manager
## read access, scoped to the exact ARNs those two Deployments need
## (DEPLOYMENT.md §6) — nothing broader, and not shared with the ALB
## controller's own role above.
## ---------------------------------------------------------------------------

# Any of app_service_accounts (guardpipe-api, guardpipe-worker) may assume
# this role — both need the same Secrets Manager reads, so one shared role
# is the established pattern here (CLAUDE.md's "one repo struct, several
# modules' interfaces" reasoning applied to IAM instead). Built via jsonencode
# rather than aws_iam_policy_document because it needs one Statement entry
# per service account name, generated with a for expression.
resource "aws_iam_role" "guardpipe_app" {
  name = "${var.project}-app"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      for sa in var.app_service_accounts : {
        Effect = "Allow"
        Principal = {
          Federated = aws_iam_openid_connect_provider.eks.arn
        }
        Action = "sts:AssumeRoleWithWebIdentity"
        Condition = {
          StringEquals = {
            "${local.oidc_provider_url_no_scheme}:aud" = "sts.amazonaws.com"
            "${local.oidc_provider_url_no_scheme}:sub" = "system:serviceaccount:${var.app_namespace}:${sa}"
          }
        }
      }
    ]
  })
}

data "aws_iam_policy_document" "guardpipe_app_secrets" {
  statement {
    actions = ["secretsmanager:GetSecretValue"]
    resources = [
      data.terraform_remote_state.persistent.outputs.jwt_secret_arn,
      data.terraform_remote_state.persistent.outputs.encryption_key_secret_arn,
      data.terraform_remote_state.persistent.outputs.gemini_api_key_secret_arn,
      data.terraform_remote_state.persistent.outputs.rds_master_user_secret_arn,
    ]
  }
}

resource "aws_iam_policy" "guardpipe_app_secrets" {
  name   = "${var.project}-app-secrets"
  policy = data.aws_iam_policy_document.guardpipe_app_secrets.json
}

resource "aws_iam_role_policy_attachment" "guardpipe_app_secrets" {
  role       = aws_iam_role.guardpipe_app.name
  policy_arn = aws_iam_policy.guardpipe_app_secrets.arn
}

## ---------------------------------------------------------------------------
## Cost guardrail (DEPLOYMENT.md §3/§13.3) — alerts at 80% actual spend and
## 100% forecasted spend against the monthly limit, so a mistake left running
## surfaces before it becomes a surprise bill.
## ---------------------------------------------------------------------------

resource "aws_budgets_budget" "monthly" {
  name         = "${var.project}-monthly"
  budget_type  = "COST"
  limit_amount = tostring(var.budget_limit_usd)
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 80
    threshold_type             = "PERCENTAGE"
    notification_type          = "ACTUAL"
    subscriber_email_addresses = [var.budget_alert_email]
  }

  notification {
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    notification_type          = "FORECASTED"
    subscriber_email_addresses = [var.budget_alert_email]
  }
}
