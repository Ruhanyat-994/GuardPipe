package docreview

import "github.com/Ruhanyat-994/GuardPipe/internal/domain"

// Rules is the Core rule catalogue documentation/05-module-specifications.md
// §11 lists for docreview, registered into advisory.RuleRegistry at wiring
// time (cmd/guardpipe/main.go), the same mechanism every other engine's own
// Rules slice already uses.
//
// Unlike every earlier engine, docreview has no deterministic rule
// evaluators of its own — every finding is AI-authored (aipass.go), and the
// model is told to return one of exactly these rule IDs (see
// buildCategoriesVar). This slice is what makes that AI output land on a
// real, permanent, findings.rule_id-satisfying row rather than an ID the
// model invented on the spot — a returned rule_id that isn't in this list is
// dropped, not persisted (findings.go's classifyRuleID).
//
// The last entry, docreview.security.prompt-injection-attempt, isn't one of
// the 13 Core rules in §11's table — it's the rule ID modules/ai.injectionFinding
// builds for whichever engine called it (documentation/10-ai-integration.md
// §5), and every finding's rule_id is a foreign key into this table
// (documentation/06-database-design.md §4.11), so docreview has to register
// it too, or its own injection-defence finding would fail to insert.
var Rules = []domain.RuleMeta{
	// --- quality ---
	{
		ID: "docreview.quality.spelling-grammar", Title: "Spelling or grammatical error",
		Description: "This document contains a misspelling or grammatical error.",
		Severity:    domain.SeverityInformational, Confidence: domain.ConfidenceMedium,
		Remediation: "Apply the suggested correction, or otherwise fix the flagged text.",
		References:  []string{"documentation/05-module-specifications.md §11"},
		Tier:        domain.TierCore,
	},
	{
		ID: "docreview.quality.ambiguous-requirement", Title: "Untestable, ambiguous requirement language",
		Description: "A requirement is stated using untestable language (\"fast\", \"secure\", \"user-friendly\", \"as needed\") rather than a specific, measurable criterion.",
		Severity:    domain.SeverityInformational, Confidence: domain.ConfidenceMedium,
		Remediation: "Rewrite the requirement with a specific, testable criterion (e.g. a concrete threshold, a named mechanism) in place of the ambiguous term.",
		References:  []string{"documentation/05-module-specifications.md §11"},
		Tier:        domain.TierCore,
	},

	// --- completeness gaps ---
	{
		ID: "docreview.gap.no-auth-mechanism", Title: "No authentication mechanism described",
		Description: "This is an architecture or requirements document that never states how authentication works — who a user proves themselves to be, and how.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceMedium,
		Remediation: "Add a section naming the specific authentication mechanism (e.g. password + JWT, SSO, mTLS) this system uses.",
		References:  []string{"documentation/05-module-specifications.md §11"},
		Tier:        domain.TierCore,
	},
	{
		ID: "docreview.gap.no-data-classification", Title: "No data classification stated",
		Description: "This document never states which data the system handles is sensitive (personal data, credentials, financial information, ...) and which is not.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceMedium,
		Remediation: "Add a short data classification: name each category of sensitive data this system stores or processes, and how it's protected.",
		References:  []string{"documentation/05-module-specifications.md §11"},
		Tier:        domain.TierCore,
	},
	{
		ID: "docreview.gap.no-threat-consideration", Title: "No security or threat section",
		Description: "This is a design document with no section discussing security or threats at all — not even a brief one.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceMedium,
		Remediation: "Add a threat-model or security-considerations section, even a short one, naming what this design is meant to defend against.",
		References:  []string{"documentation/05-module-specifications.md §11"},
		Tier:        domain.TierCore,
	},
	{
		ID: "docreview.gap.no-error-handling", Title: "No stated failure behaviour for a critical flow",
		Description: "A critical flow this document describes (payment, authentication, data deletion, ...) never states what happens when it fails partway through.",
		Severity:    domain.SeverityLow, Confidence: domain.ConfidenceMedium,
		Remediation: "State the failure behaviour for this flow explicitly — what the user sees, what state the system is left in, and how it recovers.",
		References:  []string{"documentation/05-module-specifications.md §11"},
		Tier:        domain.TierCore,
	},

	// --- design defects ---
	{
		ID: "docreview.design.plaintext-credentials", Title: "Documented plaintext credential storage",
		Description: "This document describes storing a credential (password, API key, token) in plaintext rather than hashed or encrypted.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceMedium,
		CWE:         []string{"CWE-256"},
		Remediation: "Hash passwords with a modern algorithm (e.g. Argon2id); encrypt other credentials at rest and document the encryption mechanism and key management.",
		References:  []string{"documentation/05-module-specifications.md §11", "OWASP: Password Storage Cheat Sheet"},
		Tier:        domain.TierCore,
	},
	{
		ID: "docreview.design.custom-crypto", Title: "Documented intent to implement custom cryptography",
		Description: "This document describes writing a custom cryptographic algorithm or protocol instead of using an established, vetted library.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceMedium,
		CWE:         []string{"CWE-327"},
		Remediation: "Replace the custom cryptography with a well-established, audited library implementing a standard algorithm.",
		References:  []string{"documentation/05-module-specifications.md §11", "OWASP: Cryptographic Storage Cheat Sheet"},
		Tier:        domain.TierCore,
	},
	{
		ID: "docreview.design.disabled-tls", Title: "Documented TLS verification bypass",
		Description: "This document describes disabling or bypassing TLS certificate verification, in production or otherwise.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceMedium,
		CWE:         []string{"CWE-295"},
		Remediation: "Never disable TLS verification outside a throwaway local-dev environment; document the specific narrower exception if one is genuinely required.",
		References:  []string{"documentation/05-module-specifications.md §11"},
		Tier:        domain.TierCore,
	},
	{
		ID: "docreview.design.public-admin-interface", Title: "Admin interface documented as internet-exposed without auth",
		Description: "This document describes an administrative interface as reachable from the public internet with no authentication described for it.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceMedium,
		CWE:         []string{"CWE-284"},
		Remediation: "Put the admin interface behind authentication at minimum, and consider restricting network reachability (VPN, allowlist, private network) as well.",
		References:  []string{"documentation/05-module-specifications.md §11"},
		Tier:        domain.TierCore,
	},
	{
		ID: "docreview.design.no-authz-model", Title: "Multi-user system with no authorisation model described",
		Description: "This document describes a system with multiple users or roles but never states who is allowed to do what.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceMedium,
		CWE:         []string{"CWE-862"},
		Remediation: "Add an authorisation model — name the roles/permissions this system has and what each is allowed to access or do.",
		References:  []string{"documentation/05-module-specifications.md §11"},
		Tier:        domain.TierCore,
	},
	{
		ID: "docreview.design.secrets-in-config", Title: "Documented practice of committing secrets to config files",
		Description: "This document describes storing secrets directly in a config file that is (or may be) committed to version control, rather than in a secret store or environment variable.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceMedium,
		CWE:         []string{"CWE-798"},
		Remediation: "Move secrets out of committed config files into environment variables or a dedicated secret store; add the config file to .gitignore if it must exist locally.",
		References:  []string{"documentation/05-module-specifications.md §11"},
		Tier:        domain.TierCore,
	},

	// --- consistency ---
	{
		ID: "docreview.consistency.contradiction", Title: "Two documents specify incompatible behaviour",
		Description: "This document states something that contradicts another document reviewed in the same scan.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceMedium,
		Remediation: "Reconcile the two documents — decide which statement is correct and update the other to match, or document why both are intentionally different.",
		References:  []string{"documentation/05-module-specifications.md §11"},
		Tier:        domain.TierCore,
	},

	// --- hygiene (Stretch) ---
	{
		ID: "docreview.hygiene.broken-link", Title: "Dead internal link or reference to a missing file",
		Description: "This document links to or references another file that doesn't exist in the reviewed set.",
		Severity:    domain.SeverityLow, Confidence: domain.ConfidenceMedium,
		Remediation: "Fix the link, or remove it if the referenced file no longer exists.",
		References:  []string{"documentation/05-module-specifications.md §11"},
		Tier:        domain.TierStretch,
	},

	// --- AI injection-defence finding (documentation/10-ai-integration.md §5) ---
	{
		ID: "docreview.security.prompt-injection-attempt", Title: "Prompt injection attempt detected in a document sent for AI review",
		Description: "Content sent to the AI analysis service for review appears to contain an attempt to override its instructions. The AI's response was discarded rather than trusted; this finding records the attempt itself as a deterministic fact about the document.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceMedium,
		Remediation: "Review the flagged document and remove any text designed to manipulate automated analysis tools.",
		References:  []string{"documentation/10-ai-integration.md §5"},
		Tier:        domain.TierCore,
	},
}

// rulesByID is built once at package init, mirroring every other engine's
// own "declare once, reuse at emit time" shape (cicdscan.rulesByID,
// k8sscan's own rule lookup).
var rulesByID = func() map[string]domain.RuleMeta {
	m := make(map[string]domain.RuleMeta, len(Rules))
	for _, r := range Rules {
		m[r.ID] = r
	}
	return m
}()

// categoryRuleIDs is Rules minus the injection-defence anchor — the set the
// AI prompt is told about and the set findings.go validates a response's
// rule_id against (see that file's classifyRuleID). The injection anchor is
// never something the model is asked to return; modules/ai raises it itself
// on a discarded response.
var categoryRuleIDs = func() []string {
	ids := make([]string, 0, len(Rules)-1)
	for _, r := range Rules {
		if r.ID == "docreview.security.prompt-injection-attempt" {
			continue
		}
		ids = append(ids, r.ID)
	}
	return ids
}()
