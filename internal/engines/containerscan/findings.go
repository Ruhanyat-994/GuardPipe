package containerscan

import (
	"strings"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/trivy"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// misconfigFinding normalises a Trivy Dockerfile/IaC misconfiguration into
// a Finding. Confidence is always high — Trivy has no SonarQube-style
// "needs review" hotspot tier for misconfigurations.
func misconfigFinding(scanID uuid.UUID, target string, m trivy.Misconfig) domain.Finding {
	ruleID := "containerscan.trivy." + m.ID
	location := domain.Location{
		Type:      domain.LocationTypeFile,
		Path:      target,
		LineStart: m.CauseMetadata.StartLine,
		LineEnd:   m.CauseMetadata.EndLine,
	}

	metadata := map[string]any{"trivy_check_id": m.ID}
	if impact, path := attackContext(m.ID); impact != "" || len(path) > 0 {
		if impact != "" {
			metadata["impact"] = impact
		}
		if len(path) > 0 {
			metadata["attack_path"] = path
		}
	}

	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineContainerScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, target, normalizeEvidence(m.Message)),
		Title:       firstNonEmpty(m.Title, m.ID),
		Description: firstNonEmpty(m.Message, m.Title),
		Severity:    severityFromTrivy(m.Severity), Confidence: domain.ConfidenceHigh,
		Location:    location,
		Remediation: firstNonEmpty(m.Resolution, "See the corresponding Trivy check for detailed remediation guidance."),
		Status:      domain.StatusOpen,
		Metadata:    metadata,
	}
}

// vulnerabilityFinding normalises a Trivy CVE match against an OS or
// language package into a Finding.
func vulnerabilityFinding(scanID uuid.UUID, imageRef string, v trivy.Vulnerability) domain.Finding {
	ruleID := "containerscan.trivy." + v.VulnerabilityID
	location := domain.Location{
		Type:        domain.LocationTypeImage,
		Image:       imageRef,
		LayerDigest: v.Layer.Digest,
		Path:        v.PkgName,
	}

	remediation := "No fixed version is available yet for this package."
	if v.FixedVersion != "" {
		remediation = "Upgrade " + v.PkgName + " to " + v.FixedVersion + " or later."
	}

	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineContainerScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, v.PkgName, normalizeEvidence(v.InstalledVersion)),
		// Trivy's own v.Title describes the CVE itself, not the affected
		// package — the same CVE routinely affects several distinct
		// packages built from one source package (e.g. util-linux's own
		// CVEs also hit bsdutils/mount/libblkid1/libuuid1/..., each a real,
		// independently-installed package with its own Finding here). Left
		// as v.Title alone, every one of those Findings reads as an
		// identical row in a list, which looks like duplicate/broken output
		// even though each is a distinct, correct finding — prefixing with
		// the package name is what makes them visibly distinct.
		Title:       v.PkgName + ": " + firstNonEmpty(v.Title, v.VulnerabilityID),
		Description: firstNonEmpty(v.Description, v.Title),
		Severity:    severityFromTrivy(v.Severity), Confidence: domain.ConfidenceHigh,
		CVE:         []string{v.VulnerabilityID},
		CWE:         normalizeCWE(v.CweIDs),
		Location:    location,
		Remediation: remediation,
		Status:      domain.StatusOpen,
		Metadata: map[string]any{
			"trivy_package": v.PkgName, "trivy_installed_version": v.InstalledVersion,
			"trivy_fixed_version": v.FixedVersion, "trivy_primary_url": v.PrimaryURL,
		},
	}
}

// secretFinding normalises a Trivy image-layer secret match into a
// Finding — a different surface than depscan's repository-checkout secret
// sweep (an image can contain a secret that never touched git), so this
// doesn't compete with depscan.secrets.* for the same namespace
// (documentation/05-module-specifications.md §8's "Secret scanning" note,
// ADR-0012).
func secretFinding(scanID uuid.UUID, imageRef, target string, s trivy.Secret) domain.Finding {
	ruleID := "containerscan.trivy." + s.RuleID
	location := domain.Location{
		Type:        domain.LocationTypeImage,
		Image:       imageRef,
		LayerDigest: s.Layer.Digest,
		Path:        target,
		LineStart:   s.StartLine,
		LineEnd:     s.EndLine,
	}

	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineContainerScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, target, s.RuleID),
		Title:       firstNonEmpty(s.Title, s.RuleID),
		Description: "Secret pattern (" + firstNonEmpty(s.Category, "unknown category") + ") found in the built image.",
		Severity:    severityFromTrivy(s.Severity), Confidence: domain.ConfidenceHigh,
		Location:    location,
		Remediation: "Remove the secret from the image and rotate it — rebuilding the image alone does not invalidate a credential already baked into a published layer.",
		Status:      domain.StatusOpen,
		Metadata:    map[string]any{"trivy_secret_category": s.Category},
	}
}

// severityFromTrivy maps Trivy's CRITICAL/HIGH/MEDIUM/LOW/UNKNOWN scale onto
// GuardPipe's five-level Severity.
func severityFromTrivy(s string) domain.Severity {
	switch strings.ToUpper(s) {
	case "CRITICAL":
		return domain.SeverityCritical
	case "HIGH":
		return domain.SeverityHigh
	case "MEDIUM":
		return domain.SeverityMedium
	case "LOW":
		return domain.SeverityLow
	default:
		return domain.SeverityInformational
	}
}

// normalizeCWE prefixes bare numeric CWE identifiers with "CWE-" — same
// normalisation engines/codescan applies, Finding.CWE is documented as the
// "CWE-89"-style form everywhere in the codebase.
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

// normalizeEvidence collapses whitespace so upstream reformatting/rewording
// doesn't spuriously change the fingerprint's evidence component more than
// a real content change would — same reasoning as engines/codescan's own
// normalizeEvidence.
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
