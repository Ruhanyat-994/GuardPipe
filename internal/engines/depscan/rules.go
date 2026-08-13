package depscan

import "github.com/Ruhanyat-994/GuardPipe/internal/domain"

// Rules is the Core rule catalogue documentation/05-module-specifications.md
// tables for depscan — registered into advisory.RuleRegistry at wiring time
// (cmd/guardpipe/main.go), the same mechanism Phase 5 built and left empty
// pending a real engine. depscan is that engine.
//
// depscan.hygiene.unmaintained is registered here (so GET /rules lists it,
// tier core, like every other rule) but Run never emits it this phase — it
// needs a package-registry client (npm/PyPI/etc. "last release date") that
// doesn't exist yet, and building one properly is its own adapter+tests,
// not a corner to cut inside this file. Stated here rather than hidden,
// the same posture Phase 5 took on NetworkTargetOnly's egress gap.
var Rules = []domain.RuleMeta{
	{
		ID: "depscan.vuln.known-cve", Title: "Dependency has a known vulnerability",
		Description: "This dependency version matches a known advisory range published for it.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Remediation: "Upgrade to a version outside the affected range. Check the advisory for the fixed version.",
		Tier:        domain.TierCore,
	},
	{
		ID: "depscan.vuln.no-fix-available", Title: "Known vulnerability has no patched version yet",
		Description: "This dependency has a known vulnerability, and the advisory lists no fixed version.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "No upgrade currently resolves this. Consider a workaround, a patched fork, or removing the dependency until a fix ships.",
		Tier:        domain.TierCore,
	},
	{
		ID: "depscan.secrets.committed-credential", Title: "Hardcoded credential committed to the repository",
		Description: "A value matching a known secret pattern (API key, token, or a name-based credential assignment) is committed in source.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceMedium,
		CWE:         []string{"CWE-798"},
		Remediation: "Revoke and rotate the credential immediately, remove it from git history, and load it from environment/secret storage instead.",
		Tier:        domain.TierCore,
	},
	{
		ID: "depscan.secrets.env-file-committed", Title: "Environment file committed to the repository",
		Description: "A .env-style file (not a .example/.sample template) is tracked in git and may contain real secrets.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceHigh,
		CWE:         []string{"CWE-798"},
		Remediation: "Remove the file from git history, add it to .gitignore, and rotate any credentials it may have contained.",
		Tier:        domain.TierCore,
	},
	{
		ID: "depscan.secrets.key-file-committed", Title: "Private key file committed to the repository",
		Description: "A file matching a private-key extension (.pem, .key, .p12, .pfx) or the conventional id_rsa name is tracked in git.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceHigh,
		CWE:         []string{"CWE-798"},
		Remediation: "Remove the key from git history and rotate it — any key that was ever committed must be considered compromised.",
		Tier:        domain.TierCore,
	},
	{
		ID: "depscan.hygiene.unmaintained", Title: "Dependency has had no upstream release in over 24 months",
		Description: "This dependency's package registry entry shows no release in the last two years.",
		Severity:    domain.SeverityInformational, Confidence: domain.ConfidenceMedium,
		Remediation: "Evaluate whether an actively maintained alternative exists.",
		Tier:        domain.TierCore,
	},
	{
		ID: "depscan.hygiene.no-lockfile", Title: "Dependency manifest has no lockfile",
		Description: "A manifest declares dependencies by range, with no lockfile pinning exact resolved versions — builds are not reproducible.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Remediation: "Commit the ecosystem's lockfile (package-lock.json, go.sum, composer.lock, etc.) so every install resolves identically.",
		Tier:        domain.TierCore,
	},
	{
		ID: "depscan.hygiene.wildcard-version", Title: "Dependency pinned to a wildcard version",
		Description: "A dependency is declared as \"*\" or \"latest\" rather than a specific version or range.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Remediation: "Pin to a specific version or a bounded range so upgrades are deliberate, not automatic on every install.",
		Tier:        domain.TierCore,
	},
}
