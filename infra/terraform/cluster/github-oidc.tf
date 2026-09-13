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
        "repo:${var.github_repo}:ref:refs/heads/main",
        "repo:${var.github_repo}:pull_request",
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
    sid = "EcrPush"
    actions = [
      "ecr:BatchCheckLayerAvailability",
      "ecr:GetDownloadUrlForLayer",
      "ecr:BatchGetImage",
      "ecr:PutImage",
      "ecr:InitiateLayerUpload",
      "ecr:UploadLayerPart",
      "ecr:CompleteLayerUpload",
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
## to separately keep in sync). Scoped to the AmazonEKSEditPolicy
## (create/update/delete workloads, no RBAC/secret-reading beyond what
## `kubectl set image`/`kubectl rollout` need) and to just the guardpipe
## namespace — not cluster-admin, matching this file's own least-privilege
## posture for IRSA above.
## ---------------------------------------------------------------------------

resource "aws_eks_access_entry" "github_actions" {
  cluster_name  = aws_eks_cluster.main.name
  principal_arn = aws_iam_role.github_actions.arn
}

resource "aws_eks_access_policy_association" "github_actions" {
  cluster_name  = aws_eks_cluster.main.name
  principal_arn = aws_iam_role.github_actions.arn
  policy_arn    = "arn:aws:eks::aws:cluster-access-policy/AmazonEKSEditPolicy"

  access_scope {
    type       = "namespace"
    namespaces = [var.app_namespace]
  }

  depends_on = [aws_eks_access_entry.github_actions]
}
