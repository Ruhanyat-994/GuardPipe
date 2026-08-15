package codescan

import (
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/sonarqube"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// issueFinding normalises a confirmed SonarQube VULNERABILITY issue into a
// Finding — Confidence is high, unlike hotspotFinding's medium, per
// documentation/05-module-specifications.md §6's normalisation table.
func issueFinding(scanID uuid.UUID, projectKey string, iss sonarqube.Issue, rule sonarqube.RuleInfo) domain.Finding {
	ruleID := "codescan.sonarqube." + iss.RuleKey
	path := componentPath(projectKey, iss.Component)
	location := domain.Location{Type: domain.LocationTypeFile, Path: path, LineStart: iss.Line, LineEnd: iss.LineEnd}

	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineCodeScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, path, normalizeEvidence(iss.Message)),
		Title:       firstNonEmpty(rule.Name, iss.Message),
		Description: iss.Message,
		Severity:    severityFromIssue(iss.Severity), Confidence: domain.ConfidenceHigh,
		CWE:         normalizeCWE(rule.CWE),
		Location:    location,
		Remediation: remediationText(rule.RemediationHTML),
		Status:      domain.StatusOpen,
		Metadata:    map[string]any{"sonarqube_issue_key": iss.Key, "sonarqube_project_key": projectKey},
	}
}

// hotspotFinding normalises a SonarQube security hotspot into a Finding —
// "needs manual review," not a confirmed vulnerability, hence
// Confidence: medium regardless of how urgent VulnerabilityProbability
// sounds (documentation/05-module-specifications.md §6).
func hotspotFinding(scanID uuid.UUID, projectKey string, hs sonarqube.Hotspot, rule sonarqube.RuleInfo) domain.Finding {
	ruleID := "codescan.sonarqube." + hs.RuleKey
	path := componentPath(projectKey, hs.Component)
	location := domain.Location{Type: domain.LocationTypeFile, Path: path, LineStart: hs.Line, LineEnd: hs.Line}

	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineCodeScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, path, normalizeEvidence(hs.Message)),
		Title:       firstNonEmpty(rule.Name, hs.Message),
		Description: hs.Message,
		Severity:    severityFromHotspotProbability(hs.VulnerabilityProbability), Confidence: domain.ConfidenceMedium,
		CWE:         normalizeCWE(rule.CWE),
		Location:    location,
		Remediation: remediationText(rule.RemediationHTML),
		Status:      domain.StatusOpen,
		Metadata:    map[string]any{"sonarqube_hotspot_key": hs.Key, "sonarqube_project_key": projectKey},
	}
}

// severityFromIssue maps SonarQube's issue severity scale onto GuardPipe's
// five-level Severity (documentation/05-module-specifications.md §6).
func severityFromIssue(s string) domain.Severity {
	switch s {
	case "BLOCKER", "CRITICAL":
		return domain.SeverityCritical
	case "MAJOR":
		return domain.SeverityHigh
	case "MINOR":
		return domain.SeverityMedium
	case "INFO":
		return domain.SeverityLow
	default:
		return domain.SeverityMedium
	}
}

// severityFromHotspotProbability maps a hotspot's vulnerabilityProbability
// one rung below the equivalent confirmed-issue severity — a hotspot is
// unconfirmed, so it's never reported as critical no matter how urgent its
// probability sounds (matches Confidence: medium in hotspotFinding).
func severityFromHotspotProbability(p string) domain.Severity {
	switch p {
	case "HIGH":
		return domain.SeverityHigh
	case "MEDIUM":
		return domain.SeverityMedium
	case "LOW":
		return domain.SeverityLow
	default:
		return domain.SeverityMedium
	}
}

// componentPath strips SonarQube's "<projectKey>:" component prefix, so
// Location.Path reads as "internal/db/user.go", matching every other
// engine's file-path convention.
func componentPath(projectKey, component string) string {
	return strings.TrimPrefix(component, projectKey+":")
}

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

// remediationText strips SonarQube's rule description down from HTML to
// plain text — Finding.Remediation is a plain string, and this is
// SonarQube-authored guidance, not AI-generated, so it must stand alone
// without AI regardless (CLAUDE.md's security posture section). A rule
// GetRule couldn't resolve (adapter fallback: RuleInfo{Key: ruleKey} only)
// still gets a usable, if generic, remediation string rather than an empty
// one.
func remediationText(html string) string {
	if html == "" {
		return "See the corresponding SonarQube rule for detailed remediation guidance."
	}
	text := htmlTagPattern.ReplaceAllString(html, " ")
	return strings.Join(strings.Fields(text), " ")
}

// normalizeCWE prefixes bare numeric SonarQube CWE identifiers with "CWE-"
// — SonarQube's rules/show response has varied across versions between
// bare numbers ("78") and already-prefixed values ("CWE-78"); Finding.CWE
// is documented as the "CWE-89"-style form everywhere else in the codebase.
func normalizeCWE(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, len(raw))
	for i, c := range raw {
		if strings.HasPrefix(c, "CWE-") {
			out[i] = c
		} else {
			out[i] = "CWE-" + c
		}
	}
	return out
}

// normalizeEvidence collapses whitespace in a SonarQube message so
// reformatting/rewording upstream doesn't spuriously change the
// fingerprint's evidence component more than a real content change would —
// same reasoning as engines/depscan's own normalizeEvidence.
func normalizeEvidence(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
