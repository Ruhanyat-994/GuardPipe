package k8sscan

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// evaluateRBAC implements the ten k8sscan.rbac.* Core rules
// (documentation/05-module-specifications.md §9). The eight rules that
// describe a property of a Role/ClusterRole's own PolicyRules (wildcard-*,
// secrets-read-cluster-wide, pod-create, escalate-bind, impersonate,
// exec-attach) are evaluated once per Role/ClusterRole manifest, independent
// of whether/how it's bound — a Role granting "*" is a finding on that Role
// regardless of which subjects can reach it (a full reachability analysis
// is the Stretch k8sscan.rbac.escalation-path rule, not this one).
// cluster-admin-binding and default-sa-bound describe the *binding* itself,
// so they're evaluated per RoleBinding/ClusterRoleBinding instead.
func evaluateRBAC(graph *ResourceGraph, scanID uuid.UUID) []domain.Finding {
	var findings []domain.Finding

	for _, m := range graph.Manifests {
		if m.Role == nil {
			continue
		}
		for i, rule := range m.Role.Rules {
			findings = append(findings, evaluatePolicyRule(scanID, m, i, rule)...)
		}
	}

	for _, m := range graph.Manifests {
		if m.RoleBinding == nil {
			continue
		}
		if f, ok := clusterAdminBindingFinding(scanID, m); ok {
			findings = append(findings, f)
		}
		if f, ok := defaultSABoundFinding(scanID, m); ok {
			findings = append(findings, f)
		}
	}

	return findings
}

func evaluatePolicyRule(scanID uuid.UUID, m Manifest, ruleIndex int, rule policyRule) []domain.Finding {
	var findings []domain.Finding
	fieldPath := fmt.Sprintf("rules[%d]", ruleIndex)

	if containsString(rule.Verbs, "*") {
		findings = append(findings, rbacFinding(scanID, m, fieldPath+".verbs",
			"k8sscan.rbac.wildcard-verbs", "This rule grants every possible verb.", strings.Join(rule.Verbs, ",")))
	}
	if containsString(rule.Resources, "*") {
		findings = append(findings, rbacFinding(scanID, m, fieldPath+".resources",
			"k8sscan.rbac.wildcard-resources", "This rule grants access to every resource type.", strings.Join(rule.Resources, ",")))
	}
	if containsString(rule.APIGroups, "*") {
		findings = append(findings, rbacFinding(scanID, m, fieldPath+".apiGroups",
			"k8sscan.rbac.wildcard-apigroups", "This rule matches every API group.", strings.Join(rule.APIGroups, ",")))
	}
	if m.Kind == "ClusterRole" && containsString(rule.Resources, "secrets") && rule.hasAnyVerb("get", "list", "watch") {
		findings = append(findings, rbacFinding(scanID, m, fieldPath,
			"k8sscan.rbac.secrets-read-cluster-wide", "This ClusterRole grants cluster-wide read access to Secrets.", strings.Join(rule.Verbs, ",")))
	}
	if containsString(rule.Resources, "pods") && containsString(rule.Verbs, "create") {
		findings = append(findings, rbacFinding(scanID, m, fieldPath,
			"k8sscan.rbac.pod-create", "This rule grants permission to create Pods.", "create pods"))
	}
	if (containsString(rule.Resources, "roles") || containsString(rule.Resources, "clusterroles")) && rule.hasAnyVerb("escalate", "bind") {
		findings = append(findings, rbacFinding(scanID, m, fieldPath,
			"k8sscan.rbac.escalate-bind", "This rule grants escalate or bind on roles.", strings.Join(rule.Verbs, ",")))
	}
	if containsString(rule.Verbs, "impersonate") {
		findings = append(findings, rbacFinding(scanID, m, fieldPath,
			"k8sscan.rbac.impersonate", "This rule grants the impersonate verb.", "impersonate"))
	}
	if (containsString(rule.Resources, "pods/exec") || containsString(rule.Resources, "pods/attach")) && containsString(rule.Verbs, "create") {
		findings = append(findings, rbacFinding(scanID, m, fieldPath,
			"k8sscan.rbac.exec-attach", "This rule grants pod exec or attach.", strings.Join(rule.Resources, ",")))
	}
	return findings
}

func (r policyRule) hasAnyVerb(verbs ...string) bool {
	for _, v := range verbs {
		if containsString(r.Verbs, v) {
			return true
		}
	}
	return false
}

func clusterAdminBindingFinding(scanID uuid.UUID, m Manifest) (domain.Finding, bool) {
	if m.RoleBinding.RoleRef.Kind != "ClusterRole" || m.RoleBinding.RoleRef.Name != "cluster-admin" {
		return domain.Finding{}, false
	}
	return rbacFinding(scanID, m, "roleRef",
		"k8sscan.rbac.cluster-admin-binding", "This binding grants the cluster-admin ClusterRole.", "cluster-admin"), true
}

func defaultSABoundFinding(scanID uuid.UUID, m Manifest) (domain.Finding, bool) {
	for i, subj := range m.RoleBinding.Subjects {
		if subj.Kind == "ServiceAccount" && subj.Name == "default" {
			return rbacFinding(scanID, m, fmt.Sprintf("subjects[%d]", i),
				"k8sscan.rbac.default-sa-bound", "This binding grants permissions to the default ServiceAccount.", "default"), true
		}
	}
	return domain.Finding{}, false
}

// rbacFinding builds one Finding for an RBAC rule hit. evidence is the
// specific value that tripped the rule (a verb list, a role name, ...) —
// part of the fingerprint's normalized_evidence so a genuinely different
// grant produces a different fingerprint, but reformatting the same YAML
// doesn't.
func rbacFinding(scanID uuid.UUID, m Manifest, fieldPath, ruleID, description, evidence string) domain.Finding {
	meta := ruleMetaByID(ruleID)
	normLoc := k8sNormalizedLocation(m, fieldPath)
	f := domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineK8sScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, normLoc, evidence),
		Title:       fmt.Sprintf("%s (%s/%s)", meta.Title, m.Kind, resourceDisplayName(m)),
		Description: description,
		Severity:    meta.Severity, Confidence: meta.Confidence,
		CWE:         meta.CWE,
		Location:    k8sLocation(m, fieldPath, "", evidence),
		Remediation: meta.Remediation,
		Status:      domain.StatusOpen,
	}
	return withAttackContext(f, ruleID, evidence)
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}
