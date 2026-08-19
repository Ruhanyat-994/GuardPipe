package cicdscan

import "github.com/Ruhanyat-994/GuardPipe/internal/domain"

// Rules is the Core rule catalogue documentation/05-module-specifications.md
// §10 lists for cicdscan — registered into advisory.RuleRegistry at wiring
// time (cmd/guardpipe/main.go), the same mechanism every other engine's own
// Rules slice already uses. The last entry,
// cicdscan.security.prompt-injection-attempt, isn't one of the 16 Core
// rules in that table — it's the rule ID modules/ai.injectionFinding builds
// for whichever engine called it (documentation/10-ai-integration.md §5),
// and every finding's rule_id is a foreign key into this table
// (documentation/06-database-design.md §4.11), so cicdscan — the first
// engine to actually call modules/ai — has to register it too, or its own
// injection-defence finding would fail to insert.
var Rules = []domain.RuleMeta{
	// --- supply-chain ---
	{
		ID: "cicdscan.supply-chain.unpinned-action", Title: "Action referenced by a mutable tag or branch",
		Description: "A `uses:` step references an action by a tag or branch name (e.g. `@v3`, `@main`) rather than a full commit SHA — the publisher can silently change what that reference points to after the fact.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "Pin the action to a full 40-character commit SHA (`uses: org/action@<sha> # v3`), not a mutable tag or branch.",
		References:  []string{"OWASP Top 10 CI/CD Security Risks: CICD-SEC-3 (Insufficient Pipeline-Based Access Controls)", "GitHub: Security hardening for GitHub Actions"},
		Tier:        domain.TierCore,
	},
	{
		ID: "cicdscan.supply-chain.unverified-action", Title: "Action from an unrecognised publisher",
		Description: "A `uses:` step references an action whose publisher isn't one of the well-known, widely-trusted namespaces (GitHub's own, the major cloud providers, or the action's own repository) — it may still be legitimate, but it wasn't possible to confirm that automatically.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceMedium,
		Remediation: "Confirm the action's publisher is who you expect before relying on it, or fork/vendor a copy you control.",
		References:  []string{"OWASP Top 10 CI/CD Security Risks: CICD-SEC-3 (Insufficient Pipeline-Based Access Controls)"},
		Tier:        domain.TierCore,
	},
	{
		ID: "cicdscan.supply-chain.curl-pipe-shell", Title: "Piping a downloaded script directly into a shell",
		Description: "A `run:` step downloads a script with curl/wget and pipes it straight into `bash`/`sh` without ever inspecting or pinning it — anything the remote server (or a man-in-the-middle) chooses to serve at that moment runs with this job's full permissions.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "Download the script to a file first, pin it by checksum or commit, review it, then execute the reviewed copy.",
		References:  []string{"OWASP Top 10 CI/CD Security Risks: CICD-SEC-3 (Insufficient Pipeline-Based Access Controls)"},
		Tier:        domain.TierCore,
	},
	{
		ID: "cicdscan.supply-chain.unpinned-install", Title: "Package installed with no version pin",
		Description: "A `run:` step installs a package with no version constraint and no lockfile in play — the exact code that ends up running is whatever the registry happens to serve that day.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceMedium,
		Remediation: "Pin an exact version (or commit to using the repository's own lockfile) for every package installed in CI.",
		References:  []string{"OWASP Top 10 CI/CD Security Risks: CICD-SEC-3 (Insufficient Pipeline-Based Access Controls)"},
		Tier:        domain.TierCore,
	},

	// --- trigger ---
	{
		ID: "cicdscan.trigger.pull-request-target-checkout", Title: "pull_request_target workflow checks out the pull request's own head",
		Description: "This workflow triggers on pull_request_target (which runs with the base repository's secrets and a write-capable token, precisely so a maintainer can safely react to a fork's PR) but then checks out the pull request's *own* head commit — any external contributor's code now runs with those secrets and that token.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceHigh,
		Remediation: "Either switch the trigger to pull_request (no privileged secrets), or if pull_request_target is genuinely required, never check out or execute the pull request's own code inside it.",
		References:  []string{"GitHub: Keeping your GitHub Actions and workflows secure — Preventing pwn requests", "OWASP Top 10 CI/CD Security Risks: CICD-SEC-4 (Poisoned Pipeline Execution)"},
		Tier:        domain.TierCore,
	},
	{
		ID: "cicdscan.trigger.workflow-run-untrusted", Title: "workflow_run consumes an artifact from the triggering run",
		Description: "This workflow triggers on workflow_run (which, like pull_request_target, runs with the base repository's privileges) and appears to download an artifact produced by that triggering run — if the triggering workflow can be influenced by an external contributor, so can the artifact's contents.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceMedium,
		Remediation: "Treat any artifact from the triggering run as untrusted input — validate its contents before using them, and never execute it directly.",
		References:  []string{"OWASP Top 10 CI/CD Security Risks: CICD-SEC-4 (Poisoned Pipeline Execution)"},
		Tier:        domain.TierCore,
	},
	{
		ID: "cicdscan.injection.script-injection", Title: "Untrusted event data interpolated directly into a shell command",
		Description: "A `run:` step interpolates `${{ github.event.* }}` directly into the shell command GitHub Actions builds — that value is attacker-controlled text (a PR title, an issue body, a branch name, ...) and is substituted in *before* the shell ever sees it, so it can break out of the intended command entirely.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceHigh,
		CWE:         []string{"CWE-78"},
		Remediation: "Pass the value through an `env:` variable instead and reference it as `$VARNAME` inside the script — the shell then sees it as inert data, never as command syntax.",
		References:  []string{"GitHub: Security hardening for GitHub Actions — Understanding the risk of script injections", "OWASP Top 10 CI/CD Security Risks: CICD-SEC-4 (Poisoned Pipeline Execution)"},
		Tier:        domain.TierCore,
	},

	// --- permissions ---
	{
		ID: "cicdscan.permissions.missing-block", Title: "No top-level permissions: block",
		Description: "This workflow declares no top-level `permissions:` block, so every job's GITHUB_TOKEN inherits the repository's default token permissions — which for many repositories is still broad read/write access, not the least-privilege default GitHub now recommends declaring explicitly.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceHigh,
		Remediation: "Add a top-level `permissions:` block scoped to exactly what the workflow's jobs need — `contents: read` is a safe default to start from.",
		References:  []string{"GitHub: Security hardening for GitHub Actions — Use the minimal permissions for GITHUB_TOKEN"},
		Tier:        domain.TierCore,
	},
	{
		ID: "cicdscan.permissions.write-all", Title: "permissions: write-all grants every scope write access",
		Description: "`permissions: write-all` gives the GITHUB_TOKEN write access to every available scope — contents, packages, issues, pull-requests, and more — regardless of what the workflow's jobs actually do.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "Replace write-all with an explicit per-scope block naming only the specific scopes that need write access.",
		References:  []string{"GitHub: Security hardening for GitHub Actions — Use the minimal permissions for GITHUB_TOKEN"},
		Tier:        domain.TierCore,
	},
	{
		ID: "cicdscan.permissions.excessive-token", Title: "contents: write granted where the job has no step that appears to write",
		Description: "This job's permissions grant contents: write, but none of its steps look like they push, commit, create a release, or otherwise write back to the repository — a broader grant than the job appears to need. This is a heuristic over the job's step names/actions, not proof the token is unused.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceLow,
		Remediation: "If this job genuinely never writes to the repository, narrow its permissions to contents: read.",
		References:  []string{"GitHub: Security hardening for GitHub Actions — Use the minimal permissions for GITHUB_TOKEN"},
		Tier:        domain.TierCore,
	},

	// --- secrets ---
	{
		ID: "cicdscan.secrets.echoed", Title: "Secret referenced in an echo/print step",
		Description: "A `run:` step passes a `${{ secrets.* }}` value straight to echo/print — GitHub Actions masks *known* secret values in logs, but this still writes the secret into a place logs are the most likely to end up copy-pasted, screenshotted, or otherwise leaked from, and won't be masked at all if the value has been transformed first.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceHigh,
		CWE:         []string{"CWE-532"},
		Remediation: "Never echo/print a secret, even for debugging — remove the step, or replace it with a check that only reports whether the secret is set, not its value.",
		References:  []string{"GitHub: Security hardening for GitHub Actions — Using secrets"},
		Tier:        domain.TierCore,
	},
	{
		ID: "cicdscan.secrets.inherit", Title: "secrets: inherit on a reusable workflow call",
		Description: "This job calls a reusable workflow with `secrets: inherit`, forwarding every secret this workflow has access to — including ones the called workflow may never need — rather than the specific secrets it actually requires.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceHigh,
		Remediation: "Pass only the specific secrets the reusable workflow declares needing, by name, instead of inherit.",
		References:  []string{"GitHub: Reusing workflows — Passing secrets to reusable workflows"},
		Tier:        domain.TierCore,
	},
	{
		ID: "cicdscan.secrets.in-condition", Title: "Secret referenced in an if: condition",
		Description: "An `if:` expression references `secrets.*` directly — GitHub Actions evaluates and logs the condition's result, which can leak whether (and sometimes how) a secret's value compares against something, even though the secret's raw value itself stays masked.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceMedium,
		Remediation: "Move the secret-dependent check into a step that runs and produces a boolean output instead, and branch on that output in the if: condition.",
		References:  []string{"GitHub: Security hardening for GitHub Actions — Using secrets"},
		Tier:        domain.TierCore,
	},

	// --- runner ---
	{
		ID: "cicdscan.runner.self-hosted-public", Title: "Self-hosted runner on a fork-triggerable workflow",
		Description: "This workflow can be triggered by a fork (its `on:` includes pull_request or a similarly fork-reachable event) and runs on a self-hosted runner — an external contributor's workflow run executes directly on infrastructure you control, not GitHub's own ephemeral, isolated runners.",
		Severity:    domain.SeverityCritical, Confidence: domain.ConfidenceMedium,
		Remediation: "Use GitHub-hosted runners for any workflow a fork can trigger, or gate the self-hosted job behind an approval/label step a maintainer controls.",
		References:  []string{"GitHub: Security hardening for GitHub Actions — Hardening for self-hosted runners"},
		Tier:        domain.TierCore,
	},
	{
		ID: "cicdscan.runner.unpinned-image", Title: "Container job image referenced by a mutable tag",
		Description: "This job's `container:` runs an image referenced by a mutable tag (e.g. `:latest`, `:18`) rather than a content digest — the exact image contents at run time can change without the workflow file itself changing.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceMedium,
		Remediation: "Pin the container image by its `@sha256:` digest, not a mutable tag.",
		References:  []string{"OWASP Top 10 CI/CD Security Risks: CICD-SEC-3 (Insufficient Pipeline-Based Access Controls)"},
		Tier:        domain.TierCore,
	},
	{
		ID: "cicdscan.artifact.upload-sensitive", Title: "Uploaded artifact path matches a secret-shaped pattern",
		Description: "An artifact-upload step's path matches a pattern commonly used for secrets or credentials (.env, *.pem, *.key, id_rsa, credentials*) — an uploaded artifact is downloadable by anyone with read access to the repository (or, for a public repository, anyone at all), for as long as the artifact's retention period lasts.",
		Severity:    domain.SeverityHigh, Confidence: domain.ConfidenceMedium,
		Remediation: "Exclude secret-shaped paths from the artifact, or if the file is genuinely needed downstream, pass it through a secret store instead of a build artifact.",
		References:  []string{"GitHub: Security hardening for GitHub Actions — Using secrets"},
		Tier:        domain.TierCore,
	},

	// --- AI semantic pass (documentation/05-module-specifications.md §10's
	// "AI semantic pass") ---
	{
		ID: "cicdscan.ai.semantic-finding", Title: "AI-identified workflow security concern",
		Description: "Gemini's semantic review of this workflow flagged a concern the 16 deterministic rules above didn't catch. The AI's own specific judgement is in this finding's title/description — this rule ID is a fixed, pre-registered anchor every such finding shares (an AI response can't safely propose its own new rule_id at runtime), not a description of what was actually found.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceMedium,
		Remediation: "Review the AI's specific reasoning (this finding's description) and judge whether it identifies a real issue — AI findings are a supplement to the rule pass, not independently authoritative.",
		References:  []string{"documentation/05-module-specifications.md §10"},
		Tier:        domain.TierCore,
	},

	// --- AI injection-defence finding (documentation/10-ai-integration.md §5) ---
	{
		ID: "cicdscan.security.prompt-injection-attempt", Title: "Prompt injection attempt detected in workflow content sent for AI review",
		Description: "Content sent to the AI analysis service for semantic review appears to contain an attempt to override its instructions. The AI's response was discarded rather than trusted; this finding records the attempt itself as a deterministic fact about the repository.",
		Severity:    domain.SeverityMedium, Confidence: domain.ConfidenceMedium,
		Remediation: "Review the flagged content and remove any text designed to manipulate automated analysis tools.",
		References:  []string{"documentation/10-ai-integration.md §5"},
		Tier:        domain.TierCore,
	},
}
