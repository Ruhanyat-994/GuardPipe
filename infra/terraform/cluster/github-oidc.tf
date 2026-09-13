## ---------------------------------------------------------------------------
## GitHub Actions OIDC (DEPLOYMENT.md §6/§8, DEPLOYMENT-NEXT-STEPS.md Part 9)
##
## Lets .github/workflows/deploy.yml (push to main: build, push to ECR,
## kubectl rollout) and .github/workflows/infra.yml (PR: `terraform plan`
## only — `apply`/`destroy` stay a human running workflow_dispatch by hand,
## per DEPLOYMENT.md §8's own "one click each way, not something every push
## should trigger") assume a short-lived AWS role via GitHub's own OIDC
## issuer — no long-lived AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY ever sits
## in GitHub Secrets. Separate from the aws_iam_openid_connect_provider.eks
## resource in main.tf: that one is EKS's own issuer, used for IRSA (pods
## assuming roles); this is GitHub's issuer, used for CI assuming a role.
## Two different trust relationships, deliberately not conflated.
## ---------------------------------------------------------------------------

data "tls_certificate" "github_actions_oidc" {
  url = "https://token.actions.githubusercontent.com"
}

resource "aws_iam_openid_connect_provider" "github_actions" {
  url             = "https://token.actions.githubusercontent.com"
  client_id_list  = ["sts.amazonaws.com"]
  thumbprint_list = [data.tls_certificate.github_actions_oidc.certificates[0].sha1_fingerprint]
}

# Scoped to exactly two GitHub-token shapes this repo's own workflows can
# ever present (DEPLOYMENT.md §6: "trusts only tokens from this specific
# repo's workflows, scoped to exactly the actions the pipeline needs") —
# not a bare `repo:${var.github_repo}:*`, which would trust a token from
# *any* branch or PR in this repo, not just the ones that actually deploy:
#   - a push-triggered run on main (deploy.yml)
#   - a pull_request-triggered run from any branch (infra.yml's plan job)
# workflow_dispatch (infra.yml's manual apply/destroy) is run by a human
# from the GitHub UI on whichever branch they pick — almost always main —
# so it's covered by the same ref:refs/heads/main pattern, not a third one.
#
# Wildcards after the owner/repo names, not exact matches: confirmed live
# via CloudTrail (2026-09-13, after the first real deploy.yml run failed
# with "Not authorized to perform sts:AssumeRoleWithWebIdentity" despite
# this trust policy looking correct) that GitHub's actual `sub` claim is
# "repo:Ruhanyat-994@110297704/GuardPipe@1315479864:ref:refs/heads/main" —
# GitHub appends each org/repo's own immutable numeric ID after `@`, not
# just the plain "owner/repo" name docs/examples usually show. `${var.github_repo}`
# (still just "owner/repo") gets its `/` turned into `@*/` so the wildcard
# lands in the right place for either half.
data "aws_iam_policy_document" "github_actions_assume_role" {
  statement {
    actions = ["sts:AssumeRoleWithWebIdentity"]
    principals {
      type        = "Federated"
      identifiers = [aws_iam_openid_connect_provider.github_actions.arn]
    }
    condition {
      test     = "StringEquals"
      variable = "token.actions.githubusercontent.com:aud"
      values   = ["sts.amazonaws.com"]
    }
    condition {
      test     = "StringLike"
      variable = "token.actions.githubusercontent.com:sub"
      values = [
        "repo:${replace(var.github_repo, "/", "@*/")}@*:ref:refs/heads/main",
        "repo:${replace(var.github_repo, "/", "@*/")}@*:pull_request",
      ]
    }
  }
}

resource "aws_iam_role" "github_actions" {
  name               = "${var.project}-github-actions"
  assume_role_policy = data.aws_iam_policy_document.github_actions_assume_role.json
}

# ECR push (deploy.yml) — GetAuthorizationToken has no resource-level
# permission in ECR's own action reference, it must be "*"; every other
# action is scoped to just this project's two repos, not every ECR repo on
# the account.
data "aws_iam_policy_document" "github_actions_permissions" {
  statement {
    sid       = "EcrAuth"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }

  statement {
    # "EcrPush", but also the read-only actions infra.yml needs beyond
    # pushing: DescribeImages ("find the most recently pushed tag" for the
    # cluster's first deploy) and DescribeRepositories (persistent/'s own
    # terraform plan refreshing state for aws_ecr_repository.images) — both
    # missing originally because only deploy.yml's push path was accounted
    # for; confirmed missing live 2026-09-13 by both a failing infra.yml run
    # and a failing PR-triggered `plan (persistent)` check.
    sid = "EcrPushAndRead"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
      "ecr:PutImage",
      "ecr:InitiateLayerUpload",
      "ecr:UploadLayerPart",
      "ecr:CompleteLayerUpload",
      "ecr:DescribeImages",
      "ecr:DescribeRepositories",
      "ecr:ListTagsForResource",
    ]
    resources = [
      for name in ["guardpipe", "guardpipe-web"] :
      "arn:aws:ecr:${var.aws_region}:${data.aws_caller_identity.current.account_id}:repository/${name}"
    ]
  }

  # Lets the workflow run `aws eks update-kubeconfig` to get a real endpoint
  # + CA cert for kubectl — cluster access itself is granted separately
  # below (aws_eks_access_entry), IAM alone doesn't imply any Kubernetes
  # RBAC permission.
  statement {
    sid       = "EksDescribe"
    actions   = ["eks:DescribeCluster"]
    resources = [aws_eks_cluster.main.arn]
  }

  # Everything below this line is for infra.yml's start/stop jobs, which run
  # a real `terraform apply`/`destroy` against this whole cluster/ config —
  # a materially bigger ask than DEPLOYMENT.md §6's original "ECR push,
  # eks:DescribeCluster, apply the k8s manifests" scoping anticipated.
  # Confirmed live (2026-09-13): the first real infra.yml run failed at
  # `terraform init` itself with a 403 reading the S3 state object, because
  # this role had none of what follows. Scoped to exactly the resource
  # *types* cluster/'s own Terraform manages, by name prefix wherever the
  # service supports it (IAM roles/policies under guardpipe-*, this one EKS
  # cluster and its sub-resources) — `*` only for the handful of AWS APIs
  # that genuinely have no resource-level ARN support for these actions
  # (EC2 launch templates, Budgets, listing KMS aliases).

  # Terraform's own remote state (S3 + DynamoDB lock) — cluster/ reads
  # persistent/'s state too (terraform_remote_state), so both keys, not just
  # cluster/'s own.
  statement {
    sid       = "TerraformStateBucket"
    actions   = ["s3:ListBucket"]
    resources = ["arn:aws:s3:::${var.state_bucket}"]
  }
  statement {
    sid     = "TerraformStateObjects"
    actions = ["s3:GetObject", "s3:PutObject"]
    resources = [
      "arn:aws:s3:::${var.state_bucket}/cluster/terraform.tfstate",
      "arn:aws:s3:::${var.state_bucket}/persistent/terraform.tfstate",
    ]
  }
  statement {
    sid       = "TerraformStateLock"
    actions   = ["dynamodb:GetItem", "dynamodb:PutItem", "dynamodb:DeleteItem"]
    resources = ["arn:aws:dynamodb:${var.aws_region}:${data.aws_caller_identity.current.account_id}:table/${var.state_dynamodb_table}"]
  }

  # IAM roles/policies this config manages — every one of them is named
  # "${var.project}-*" (main.tf/github-oidc.tf), so a name-prefixed ARN
  # covers all of them without a broader `iam:*` grant.
  statement {
    sid = "ManageProjectIamRolesAndPolicies"
    actions = [
      "iam:CreateRole", "iam:DeleteRole", "iam:GetRole", "iam:TagRole", "iam:UntagRole",
      "iam:ListRoleTags", "iam:ListRolePolicies", "iam:ListInstanceProfilesForRole",
      "iam:PassRole",
      "iam:CreatePolicy", "iam:DeletePolicy", "iam:GetPolicy", "iam:GetPolicyVersion",
      "iam:CreatePolicyVersion", "iam:DeletePolicyVersion", "iam:ListPolicyVersions",
      "iam:ListPolicyTags", "iam:TagPolicy", "iam:UntagPolicy",
      "iam:AttachRolePolicy", "iam:DetachRolePolicy", "iam:ListAttachedRolePolicies",
      "iam:UpdateAssumeRolePolicy",
    ]
    resources = [
      "arn:aws:iam::${data.aws_caller_identity.current.account_id}:role/${var.project}-*",
      "arn:aws:iam::${data.aws_caller_identity.current.account_id}:policy/${var.project}-*",
    ]
  }
  # AWS-managed policies this config attaches (AmazonEKSClusterPolicy etc.)
  # — not project-prefixed, so scoped by their own fixed ARN space instead.
  statement {
    sid       = "AttachAwsManagedPolicies"
    actions   = ["iam:AttachRolePolicy", "iam:DetachRolePolicy"]
    resources = ["arn:aws:iam::aws:policy/*"]
  }
  # OIDC providers (EKS's own + GitHub's) — no name-prefix support on this
  # resource type's ARN, scoped to the account's OIDC provider space instead
  # of iam:* broadly.
  statement {
    sid = "ManageOidcProviders"
    actions = [
      "iam:CreateOpenIDConnectProvider", "iam:DeleteOpenIDConnectProvider",
      "iam:GetOpenIDConnectProvider", "iam:TagOpenIDConnectProvider", "iam:UntagOpenIDConnectProvider",
      "iam:UpdateOpenIDConnectProviderThumbprint",
    ]
    resources = ["arn:aws:iam::${data.aws_caller_identity.current.account_id}:oidc-provider/*"]
  }

  # The EKS cluster itself, its node group, addons, and access entries.
  statement {
    sid = "ManageEksCluster"
    actions = [
      "eks:CreateCluster", "eks:DeleteCluster", "eks:UpdateClusterConfig", "eks:UpdateClusterVersion",
      "eks:TagResource", "eks:UntagResource", "eks:ListTagsForResource",
      "eks:CreateNodegroup", "eks:DeleteNodegroup", "eks:DescribeNodegroup", "eks:UpdateNodegroupConfig", "eks:UpdateNodegroupVersion",
      "eks:CreateAddon", "eks:DeleteAddon", "eks:DescribeAddon", "eks:UpdateAddon",
      "eks:CreateAccessEntry", "eks:DeleteAccessEntry", "eks:DescribeAccessEntry", "eks:UpdateAccessEntry",
      "eks:AssociateAccessPolicy", "eks:DisassociateAccessPolicy", "eks:ListAssociatedAccessPolicies",
    ]
    resources = [
      aws_eks_cluster.main.arn,
      "arn:aws:eks:${var.aws_region}:${data.aws_caller_identity.current.account_id}:nodegroup/${var.cluster_name}/*",
      "arn:aws:eks:${var.aws_region}:${data.aws_caller_identity.current.account_id}:addon/${var.cluster_name}/*",
      "arn:aws:eks:${var.aws_region}:${data.aws_caller_identity.current.account_id}:access-entry/${var.cluster_name}/*",
    ]
  }

  # EC2 launch template + describe calls the node group's launch template
  # needs — the EC2 API doesn't support resource-level ARN conditions for
  # Create/Describe on this resource type, so this is genuinely `*`, not a
  # scoping shortcut.
  statement {
    sid = "ManageNodeLaunchTemplate"
    actions = [
      "ec2:CreateLaunchTemplate", "ec2:DeleteLaunchTemplate", "ec2:CreateLaunchTemplateVersion",
      "ec2:DescribeLaunchTemplates", "ec2:DescribeLaunchTemplateVersions", "ec2:ModifyLaunchTemplate",
      "ec2:CreateTags", "ec2:DescribeTags",
    ]
    resources = ["*"]
  }

  # Budgets and the SSM-key-alias lookup (guardpipe_app_secrets' data
  # "aws_kms_alias" "ssm") — neither API supports resource-level ARNs for
  # these actions.
  statement {
    sid       = "ManageBudget"
    actions   = ["budgets:ViewBudget", "budgets:ModifyBudget", "budgets:ListTagsForResource", "budgets:TagResource", "budgets:UntagResource"]
    resources = ["*"]
  }
  statement {
    sid       = "ReadKmsAlias"
    actions   = ["kms:ListAliases", "kms:DescribeKey"]
    resources = ["*"]
  }

  # Read-only refresh permissions for the persistent/ layer's own PR-triggered
  # `terraform plan` (infra.yml's plan job matrixes over both layers on every
  # PR touching infra/terraform/**, even though only cluster/ is ever applied
  # by this role — see DEPLOYMENT.md §8/§9). persistent/ is applied by hand,
  # never by CI, so this is read-only, no Create/Update/Delete. Confirmed
  # missing live 2026-09-13 — a real PR's `plan (persistent)` check failed on
  # every one of these actions.
  statement {
    sid = "ReadPersistentLayerForPlan"
    actions = [
      "ec2:DescribeAvailabilityZones",
      "ec2:DescribeVpcs",
      "ec2:DescribeInternetGateways",
      "ec2:DescribeSubnets",
      "ec2:DescribeRouteTables",
      "ec2:DescribeSecurityGroups",
      "rds:DescribeDBInstances",
      "rds:DescribeDBSubnetGroups",
      "ecr:GetLifecyclePolicy",
    ]
    resources = ["*"]
  }
  statement {
    sid     = "ReadPersistentLayerSsmParams"
    actions = ["ssm:GetParameter"]
    resources = [
      data.terraform_remote_state.persistent.outputs.jwt_secret_parameter_arn,
      data.terraform_remote_state.persistent.outputs.encryption_key_parameter_arn,
      data.terraform_remote_state.persistent.outputs.gemini_api_key_parameter_arn,
    ]
  }
}

resource "aws_iam_policy" "github_actions" {
  name   = "${var.project}-github-actions"
  policy = data.aws_iam_policy_document.github_actions_permissions.json
}

resource "aws_iam_role_policy_attachment" "github_actions" {
  role       = aws_iam_role.github_actions.name
  policy_arn = aws_iam_policy.github_actions.arn
}

data "aws_caller_identity" "current" {}

## ---------------------------------------------------------------------------
## Kubernetes-side access for the same role (EKS access entries — main.tf's
## access_config sets authentication_mode = "API" specifically so this is
## the one and only way anything gets cluster access, no aws-auth ConfigMap
## to separately keep in sync).
##
## AmazonEKSClusterAdminPolicy, cluster-wide — not the narrower split this
## started as. Tried AmazonEKSEditPolicy (namespace-scoped to guardpipe) +
## AmazonEKSViewPolicy (cluster-wide, read-only) first; both failed against
## real AWS, for two different reasons, each confirmed live rather than
## guessed:
##   1. `nodes` is a cluster-scoped resource type — no namespace-scoped
##      policy can ever grant access to it, regardless of which policy.
##      infra.yml no longer needs this specifically (its node-group wait now
##      uses `aws eks wait nodegroup-active`, an IAM-only check — see the
##      comment on that step), but the underlying limitation is general.
##   2. AmazonEKSViewPolicy mirrors Kubernetes' built-in `view` ClusterRole,
##      which is read-only and — separately from (1) — this workflow needs
##      to *write* cluster-scoped objects too: cert-manager/the Secrets
##      Store CSI driver/the ALB controller each install CRDs and
##      ClusterRoles, into cert-manager/kube-system, not just the guardpipe
##      namespace. No combination short of cluster-admin covers that.
## This role can already create/destroy the entire cluster and every IAM
## resource around it via Terraform (see the permissions policy above) —
## Kubernetes cluster-admin on top of that is not a meaningfully larger
## blast radius, just the same trust boundary expressed in the other API.
## ---------------------------------------------------------------------------

resource "aws_eks_access_entry" "github_actions" {
  cluster_name  = aws_eks_cluster.main.name
  principal_arn = aws_iam_role.github_actions.arn
}

resource "aws_eks_access_policy_association" "github_actions_admin" {
  cluster_name  = aws_eks_cluster.main.name
  principal_arn = aws_iam_role.github_actions.arn
  policy_arn    = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSClusterAdminPolicy"

  access_scope {
    type = "cluster"
  }

  depends_on = [aws_eks_access_entry.github_actions]
}
