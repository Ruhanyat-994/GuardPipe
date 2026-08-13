package depscan

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/advisory"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

func (e *Engine) noLockfileFinding(scanID uuid.UUID, result parseResult) domain.Finding {
	const ruleID = "depscan.hygiene.no-lockfile"
	location := domain.Location{Type: domain.LocationTypeFile, Path: result.ManifestPath}
	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineDepScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, result.ManifestPath, ruleID),
		Title:       "Dependency manifest has no lockfile",
		Description: fmt.Sprintf("%s declares dependencies with no lockfile pinning exact resolved versions.", result.ManifestPath),
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Location:    location,
		Remediation: "Commit the ecosystem's lockfile so every install resolves identically.",
		Status:      domain.StatusOpen,
	}
}

func (e *Engine) wildcardVersionFinding(scanID uuid.UUID, dep Dependency) domain.Finding {
	const ruleID = "depscan.hygiene.wildcard-version"
	normLoc := dep.Ecosystem + "|" + dep.Name + "|" + dep.ManifestPath
	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineDepScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, normLoc, dep.DeclaredRange),
		Title:       fmt.Sprintf("%s is pinned to a wildcard version", dep.Name),
		Description: fmt.Sprintf("%s declares %s as %q — any install may resolve to a different version.", dep.ManifestPath, dep.Name, dep.DeclaredRange),
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Location: domain.Location{
			Type: domain.LocationTypeDependency, Ecosystem: dep.Ecosystem,
			Package: dep.Name, Version: dep.Version, ManifestPath: dep.ManifestPath,
		},
		Remediation: "Pin to a specific version or a bounded range.",
		Status:      domain.StatusOpen,
	}
}

func (e *Engine) knownCVEFinding(scanID uuid.UUID, dep Dependency, adv advisory.Advisory) domain.Finding {
	const ruleID = "depscan.vuln.known-cve"
	severity := severityFromCVSSVector(adv.CVSSVector)
	normLoc := dep.Ecosystem + "|" + dep.Name + "|" + adv.ID

	var cve []string
	if adv.CVE != "" {
		cve = []string{adv.CVE}
	}

	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineDepScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, normLoc, adv.ID),
		Title:       fmt.Sprintf("%s@%s has a known vulnerability (%s)", dep.Name, dep.Version, firstNonEmpty(adv.CVE, adv.ID)),
		Description: adv.Summary,
		Severity:    severity, Confidence: domain.ConfidenceHigh,
		CVE:        cve,
		CVSSVector: nonEmptyPtr(adv.CVSSVector),
		Location: domain.Location{
			Type: domain.LocationTypeDependency, Ecosystem: dep.Ecosystem,
			Package: dep.Name, Version: dep.Version, ManifestPath: dep.ManifestPath,
		},
		Remediation: remediationFor(adv),
		Status:      domain.StatusOpen,
		Metadata:    map[string]any{"osv_id": adv.ID},
	}
}

func (e *Engine) noFixAvailableFinding(scanID uuid.UUID, dep Dependency, adv advisory.Advisory) domain.Finding {
	const ruleID = "depscan.vuln.no-fix-available"
	severity := bumpSeverity(severityFromCVSSVector(adv.CVSSVector))
	normLoc := dep.Ecosystem + "|" + dep.Name + "|" + adv.ID

	var cve []string
	if adv.CVE != "" {
		cve = []string{adv.CVE}
	}

	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineDepScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, normLoc, adv.ID),
		Title:       fmt.Sprintf("%s@%s has no patched version available", dep.Name, dep.Version),
		Description: "This vulnerability has no fixed version listed in its advisory — upgrading alone will not resolve it.",
		Severity:    severity, Confidence: domain.ConfidenceHigh,
		CVE:        cve,
		CVSSVector: nonEmptyPtr(adv.CVSSVector),
		Location: domain.Location{
			Type: domain.LocationTypeDependency, Ecosystem: dep.Ecosystem,
			Package: dep.Name, Version: dep.Version, ManifestPath: dep.ManifestPath,
		},
		Remediation: "No upgrade currently resolves this. Consider a workaround, a patched fork, or removing the dependency.",
		Status:      domain.StatusOpen,
		Metadata:    map[string]any{"osv_id": adv.ID},
	}
}

func (e *Engine) secretFinding(scanID uuid.UUID, m secretMatch) domain.Finding {
	const ruleID = "depscan.secrets.committed-credential"
	normEvidence := normalizeEvidence(m.Matched)
	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineDepScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, m.Path, normEvidence),
		Title:       fmt.Sprintf("%s found in %s", m.Pattern, m.Path),
		Description: fmt.Sprintf("A value matching the %q pattern is committed in source.", m.Pattern),
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceMedium,
		CWE: []string{"CWE-798"},
		Location: domain.Location{
			Type: domain.LocationTypeFile, Path: m.Path, LineStart: m.LineStart, LineEnd: m.LineStart,
		},
		Evidence: []domain.Evidence{{
			Kind: domain.EvidenceKindCodeSnippet, Value: redactSecret(m.Matched), Redacted: true,
			LineStart: m.LineStart, LineEnd: m.LineStart,
		}},
		Remediation: "Revoke and rotate the credential immediately, remove it from git history, and load it from environment/secret storage instead.",
		Status:      domain.StatusOpen,
	}
}

func (e *Engine) envFileFinding(scanID uuid.UUID, path string) domain.Finding {
	const ruleID = "depscan.secrets.env-file-committed"
	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineDepScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, path, ruleID),
		Title:       fmt.Sprintf("Environment file %s is committed to the repository", path),
		Description: "A .env-style file is tracked in git and may contain real secrets.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceHigh,
		CWE:         []string{"CWE-798"},
		Location:    domain.Location{Type: domain.LocationTypeFile, Path: path},
		Remediation: "Remove the file from git history, add it to .gitignore, and rotate any credentials it may have contained.",
		Status:      domain.StatusOpen,
	}
}

func (e *Engine) keyFileFinding(scanID uuid.UUID, path string) domain.Finding {
	const ruleID = "depscan.secrets.key-file-committed"
	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineDepScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, path, ruleID),
		Title:       fmt.Sprintf("Private key file %s is committed to the repository", path),
		Description: "A file matching a private-key extension or the conventional id_rsa name is tracked in git.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceHigh,
		CWE:         []string{"CWE-798"},
		Location:    domain.Location{Type: domain.LocationTypeFile, Path: path},
		Remediation: "Remove the key from git history and rotate it — any key that was ever committed must be considered compromised.",
		Status:      domain.StatusOpen,
	}
}

// severityFromCVSSVector is a lightweight approximation, not a full CVSS
// 3.1 base-score calculator: it reads the vector's impact metrics
// (Confidentiality/Integrity/Availability) directly rather than computing
// the numeric score. Building a correct CVSS calculator is its own
// well-scoped subsystem; this gets a defensible severity out of the vector
// GuardPipe already has (adv.CVSSVector) without that separate project.
func severityFromCVSSVector(vector string) domain.Severity {
	if vector == "" {
		return domain.SeverityMedium
	}
	switch {
	case containsAny(vector, "/C:H", "/I:H", "/A:H"):
		return domain.SeverityHigh
	case containsAny(vector, "/C:L", "/I:L", "/A:L"):
		return domain.SeverityMedium
	default:
		return domain.SeverityMedium
	}
}

// bumpSeverity raises a severity by one level — depscan.vuln.no-fix-available's
// documented "+1 level" from its known-cve base (documentation/05-module-specifications.md's
// Core rules table).
func bumpSeverity(s domain.Severity) domain.Severity {
	switch s {
	case domain.SeverityHigh:
		return domain.SeverityCritical
	case domain.SeverityMedium:
		return domain.SeverityHigh
	case domain.SeverityLow:
		return domain.SeverityMedium
	case domain.SeverityInformational:
		return domain.SeverityLow
	default:
		return s
	}
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func nonEmptyPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func remediationFor(adv advisory.Advisory) string {
	if adv.HasFix {
		return fmt.Sprintf("Upgrade to version %s or later.", adv.FixedVersion)
	}
	return "No fixed version is listed yet. Check the advisory for a workaround."
}
