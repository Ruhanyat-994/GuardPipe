package k8sscan

import "github.com/Ruhanyat-994/GuardPipe/internal/domain"

// rulesByID is built once (package init) from Rules, so every finding
// constructor across rbac.go/workload.go/network.go can pull a rule's
// Title/Severity/Confidence/CWE/Remediation from one place instead of
// repeating it inline — the same "declare once, reuse at emit time" shape
// depscan.Rules already established.
var rulesByID = func() map[string]domain.RuleMeta {
	m := make(map[string]domain.RuleMeta, len(Rules))
	for _, r := range Rules {
		m[r.ID] = r
	}
	return m
}()

// ruleMetaByID panics on an unknown ID — a finding constructor referencing
// a RuleID that isn't in Rules is a programmer error caught immediately by
// any test that emits it, not something to handle gracefully at runtime.
func ruleMetaByID(ruleID string) domain.RuleMeta {
	meta, ok := rulesByID[ruleID]
	if !ok {
		panic("k8sscan: unknown rule ID " + ruleID + " — add it to Rules in rules.go")
	}
	return meta
}

// k8sLocation builds the domain.Location for a finding on manifest m at
// fieldPath, carrying the Helm source context (BUILD_GUIDE.md Phase 9) when
// m came from a rendered chart. LineStart is m's document-start line — real
// and clickable for a raw manifest (File is what was actually parsed);
// always 0 for a Helm-sourced one, since m.DocLine was already cleared at
// render time for exactly that reason (helm.go). Field-path-level precision
// (jumping to the exact offending line within the document, not just the
// document's start) isn't attempted — that would need a YAML-node walk
// matching every rule family's own fieldPath string shape, a much larger
// undertaking than this pass's actual goal: a working link to the right
// place in the right file, not a broken one.
func k8sLocation(m Manifest, fieldPath, container, value string) domain.Location {
	return domain.Location{
		Type: domain.LocationTypeK8s, File: m.File, Kind: m.Kind, Name: m.Name, Namespace: m.Namespace,
		Container:    container,
		FieldPath:    fieldPath,
		Value:        value,
		LineStart:    m.DocLine,
		FromHelm:     m.FromHelm,
		ChartName:    m.ChartName,
		TemplateFile: m.TemplateFile,
	}
}

// withAttackContext fills in f.Metadata's "impact"/"attack_path" keys from
// attackContext (attackpath.go) — called as the last step by every finding
// constructor in this package, right after Location, so ruleID/value need
// only be threaded through once per constructor rather than duplicated at
// every call site.
func withAttackContext(f domain.Finding, ruleID, value string) domain.Finding {
	impact, path := attackContext(ruleID, value)
	if impact == "" && len(path) == 0 {
		return f
	}
	f.Metadata = map[string]any{}
	if impact != "" {
		f.Metadata["impact"] = impact
	}
	if len(path) > 0 {
		f.Metadata["attack_path"] = path
	}
	return f
}

// k8sNormalizedLocation is the fingerprint's location component — deliberately
// kind+namespace+name+fieldPath, not File. A Kubernetes object's real
// identity in the cluster is its (namespace, kind, name), which survives a
// manifest being moved or renamed to a different file; a file-path-based
// normalisation (depscan's own convention, appropriate there since a source
// file *is* the identity) would treat that as a brand-new finding, which is
// wrong for a resource that didn't actually change.
func k8sNormalizedLocation(m Manifest, fieldPath string) string {
	return m.Kind + "|" + m.Namespace + "|" + m.Name + "|" + fieldPath
}

// resourceDisplayName is name, or namespace/name for a namespaced resource
// — used only in a Finding's human-readable Title, never in the fingerprint.
func resourceDisplayName(m Manifest) string {
	if m.Namespace == "" {
		return m.Name
	}
	return m.Namespace + "/" + m.Name
}
