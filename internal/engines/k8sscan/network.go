package k8sscan

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// evaluateNetwork implements the six k8sscan.network.*/k8sscan.secrets.*
// Core rules (documentation/05-module-specifications.md §9).
func evaluateNetwork(graph *ResourceGraph, scanID uuid.UUID) []domain.Finding {
	var findings []domain.Finding

	for _, m := range graph.Manifests {
		switch {
		case m.Pod != nil:
			findings = append(findings, noNetworkPolicyFindings(graph, scanID, m)...)
			findings = append(findings, envFromSecretFindings(scanID, m)...)
		case m.NetworkPolicy != nil:
			findings = append(findings, allowAllEgressFindings(scanID, m)...)
		case m.Service != nil:
			findings = append(findings, serviceFindings(scanID, m)...)
		case m.Secret != nil:
			if f, ok := literalSecretFinding(scanID, m); ok {
				findings = append(findings, f)
			}
		}
	}

	return findings
}

// noNetworkPolicyFindings fires once per workload whose namespace has no
// NetworkPolicy at all — a coarse, still-accurate-per-the-module-spec
// signal (real pod-selector matching would need full label-selector
// evaluation, which ResourceGraph doesn't implement). Repeated across every
// workload in an unprotected namespace is expected; Phase 13's finding
// grouping (BUILD_GUIDE.md) is what collapses that in the UI, not this
// engine holding namespace-level state itself.
func noNetworkPolicyFindings(graph *ResourceGraph, scanID uuid.UUID, m Manifest) []domain.Finding {
	if len(graph.networkPoliciesFor(m.Namespace)) > 0 {
		return nil
	}
	return []domain.Finding{workloadFinding(scanID, m, "metadata.namespace",
		"k8sscan.network.no-networkpolicy", "", "no NetworkPolicy in namespace")}
}

func allowAllEgressFindings(scanID uuid.UUID, m Manifest) []domain.Finding {
	if !containsString(m.NetworkPolicy.Spec.PolicyTypes, "Egress") {
		return nil
	}
	var findings []domain.Finding
	for i, rule := range m.NetworkPolicy.Spec.Egress {
		if !rule.unrestricted() {
			continue
		}
		findings = append(findings, networkFinding(scanID, m, fmt.Sprintf("spec.egress[%d]", i),
			"k8sscan.network.allow-all-egress", fmt.Sprintf("egress[%d]", i)))
	}
	return findings
}

func serviceFindings(scanID uuid.UUID, m Manifest) []domain.Finding {
	var findings []domain.Finding
	switch m.Service.Spec.Type {
	case "NodePort":
		findings = append(findings, networkFinding(scanID, m, "spec.type",
			"k8sscan.network.nodeport-service", "NodePort"))
	case "LoadBalancer":
		if len(m.Service.Spec.LoadBalancerSourceRanges) == 0 {
			findings = append(findings, networkFinding(scanID, m, "spec.loadBalancerSourceRanges",
				"k8sscan.network.loadbalancer-no-source-ranges", "unset"))
		}
	}
	return findings
}

func literalSecretFinding(scanID uuid.UUID, m Manifest) (domain.Finding, bool) {
	if len(m.Secret.Data) == 0 && len(m.Secret.StringData) == 0 {
		return domain.Finding{}, false
	}
	fieldPath := "data"
	if len(m.Secret.StringData) > 0 {
		fieldPath = "stringData"
	}
	return networkFinding(scanID, m, fieldPath, "k8sscan.secrets.literal-in-manifest", fieldPath+" present"), true
}

func envFromSecretFindings(scanID uuid.UUID, m Manifest) []domain.Finding {
	var findings []domain.Finding
	allContainers := append(append([]container{}, m.Pod.Containers...), m.Pod.InitContainers...)
	for _, c := range allContainers {
		for i, ef := range c.EnvFrom {
			if !ef.SecretRef.set {
				continue
			}
			fieldPath := fmt.Sprintf("spec.template.spec.containers[%s].envFrom[%d].secretRef", c.Name, i)
			findings = append(findings, networkFinding(scanID, m, fieldPath,
				"k8sscan.secrets.env-from-secret-all", "container:"+c.Name))
		}
	}
	return findings
}

func networkFinding(scanID uuid.UUID, m Manifest, fieldPath, ruleID, evidence string) domain.Finding {
	meta := ruleMetaByID(ruleID)
	normLoc := k8sNormalizedLocation(m, fieldPath)
	f := domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineK8sScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, normLoc, evidence),
		Title:       fmt.Sprintf("%s (%s/%s)", meta.Title, m.Kind, resourceDisplayName(m)),
		Description: meta.Description,
		Severity:    meta.Severity, Confidence: meta.Confidence,
		CWE:         meta.CWE,
		Location:    k8sLocation(m, fieldPath, "", evidence),
		Remediation: meta.Remediation,
		Status:      domain.StatusOpen,
	}
	return withAttackContext(f, ruleID, evidence)
}
