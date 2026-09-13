// Package reporting assembles a completed scan into a self-contained report
// and renders it as JSON, CSV, or PDF (documentation/07-api-specification.md
// §5's `GET /scans/{id}/export`, BUILD_GUIDE.md Phase 13 — export pulled
// forward ahead of scoring/triage/correlation, which this package does not
// touch). One Assembler.Build call produces one ReportData; every format
// renders from that same struct, so the three formats can never disagree
// about what a report says.
//
// ReportData itself still carries no risk-score field — modules/scoring
// exists now (feeding orchestrator.ScanDetail.Risk, which
// Assembler.attachExecutiveSummary reads for the AI prompt vars), but no
// renderer here (JSON/CSV/PDF) surfaces it in the report body yet. A report
// is honest about what's actually rendered today (finding counts, engine
// coverage, an AI-authored narrative) rather than fabricating a number no
// renderer displays.
package reporting

import (
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// ReportData is one scan's report content — everything every renderer
// (JSON/CSV/PDF) needs, already resolved to plain values (no further DB or
// service calls at render time).
type ReportData struct {
	GeneratedAt time.Time `json:"generated_at"`

	ProjectName       string `json:"project_name"`
	RepositoryOwner   string `json:"repository_owner,omitempty"`
	RepositoryName    string `json:"repository_name,omitempty"`
	RepositoryBranch  string `json:"repository_branch,omitempty"`
	PentestTargetHost string `json:"pentest_target_host,omitempty"`

	ScanID     uuid.UUID         `json:"scan_id"`
	ScanNumber int               `json:"scan_number"`
	ScanType   domain.ScanType   `json:"scan_type"`
	ScanStatus domain.ScanStatus `json:"scan_status"`
	Branch     string            `json:"branch,omitempty"`
	CommitSHA  string            `json:"commit_sha,omitempty"`
	QueuedAt   time.Time         `json:"queued_at"`
	StartedAt  *time.Time        `json:"started_at,omitempty"`
	FinishedAt *time.Time        `json:"finished_at,omitempty"`

	FindingCounts map[domain.Severity]int `json:"finding_counts"`
	TotalFindings int                     `json:"total_findings"`

	// RequestedByName/Email/IP are this scan's accountability stamp — who
	// triggered it and from where (domain.Scan.TriggeredBy/RequestedFromIP,
	// resolved to a display name/email by Assembler.Build). Empty when the
	// triggering user was since deleted or the scan predates this field —
	// every renderer treats that as "omit the line," never a placeholder.
	// This is GuardPipe's watermark: the same "who ran this and from where"
	// stamp Nessus/Qualys-class tools carry on their own exported reports,
	// so responsibility for the scan itself — was the target's owner
	// actually authorised? — stays attributable to the account that ran it,
	// not to GuardPipe.
	RequestedByName  string `json:"requested_by_name,omitempty"`
	RequestedByEmail string `json:"requested_by_email,omitempty"`
	RequestedFromIP  string `json:"requested_from_ip,omitempty"`
	// Disclaimer is fixed, not AI-authored or configurable per report —
	// every export carries the same responsibility statement regardless of
	// scan content. Not a substitute for real legal review before this
	// becomes a customer-facing SaaS term.
	Disclaimer string `json:"disclaimer"`

	Jobs     []JobSummary `json:"jobs"`
	Findings []FindingRow `json:"findings"`

	// ExecutiveSummary/TopPriorities are AI-authored (exactly one call per
	// Build, ai.PromptSummariseScan — see assembler.go's budget note). Empty
	// when AI is disabled, unavailable, or the call fails — never required:
	// every other field is real without it, and every renderer treats an
	// empty summary as "omit this section," not an error.
	ExecutiveSummary string   `json:"executive_summary,omitempty"`
	TopPriorities    []string `json:"top_priorities,omitempty"`
}

// JobSummary is one engine's contribution to the report — what ran, its
// outcome, and (pentest only) what it actually checked, so a clean job
// still has something real to show beyond a finding count of zero.
type JobSummary struct {
	Engine       domain.EngineID  `json:"engine"`
	Status       domain.JobStatus `json:"status"`
	StartedAt    *time.Time       `json:"started_at,omitempty"`
	FinishedAt   *time.Time       `json:"finished_at,omitempty"`
	ErrorReason  string           `json:"error_reason,omitempty"`
	SkipReason   string           `json:"skip_reason,omitempty"`
	FindingCount int              `json:"finding_count"`
	Coverage     *PentestCoverage `json:"coverage,omitempty"`

	// ValidatedSubdomains/ValidatedDirectories give a basic pentest report
	// the concrete "what did enumeration actually confirm exists" lists a
	// finding-only view doesn't — a subdomain only appears here once
	// AssetValidation has re-resolved it (dropped otherwise, the same
	// "validated, not just guessed" standard the private/loopback/metadata
	// check already applies to the root target), and a directory only
	// appears once a disclosure/admin-path ffuf run got a real 200 response
	// for it (ffuf's own status filter already excludes 403/404 near-misses
	// — a raw wordlist guess that returned nothing is never listed). Neither
	// implies a vulnerability on its own; each may also separately appear in
	// Findings above when it happens to match a specific security-relevant
	// rule (e.g. an exposed .git/.env/backup file, or a name matching a
	// sensitive-subdomain pattern) — these two lists are the broader,
	// unfiltered enumeration result, not a second copy of Findings. Pentest
	// jobs only; nil for every other engine.
	ValidatedSubdomains  []string `json:"validated_subdomains,omitempty"`
	ValidatedDirectories []string `json:"validated_directories,omitempty"`
}

// PentestCoverage mirrors internal/engines/pentest.Coverage field-for-field
// (same json tags), decoded from the job's persisted Stats map rather than
// importing that engine package directly — reporting depends on the shared
// Finding/Scan model, not on one engine's internal types
// (documentation/03-architecture-overview.md §6.2's dependency rule).
type PentestCoverage struct {
	OpenPorts           []int    `json:"open_ports"`
	HTTPServicesFound   int      `json:"http_services_found"`
	TLSPortsChecked     int      `json:"tls_ports_checked"`
	TechnologiesFound   []string `json:"technologies_found,omitempty"`
	NucleiCategoriesRun []string `json:"nuclei_categories_run"`
	CrawledPathsFound   int      `json:"crawled_paths_found"`
	// SubdomainsFound is meaningful only when "subdomain_enum" appears in
	// PhasesCompleted below — 0 there means "ran, found nothing," 0 with
	// the phase absent from PhasesCompleted means "didn't run" (disabled
	// via PentestScanConfig.SubdomainEnum).
	SubdomainsFound int      `json:"subdomains_found"`
	TotalScriptRuns int      `json:"total_script_runs"`
	PhasesCompleted []string `json:"phases_completed"`
	PhasesSkipped   []string `json:"phases_skipped,omitempty"`
}

// FindingRow is one Finding flattened for a report — Location is
// pre-formatted to a single human-readable string (FormatLocation) so every
// renderer shows it identically instead of three slightly different
// re-implementations of "how do I print a Location."
type FindingRow struct {
	Engine      domain.EngineID   `json:"engine"`
	RuleID      string            `json:"rule_id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Severity    domain.Severity   `json:"severity"`
	Confidence  domain.Confidence `json:"confidence"`
	CWE         []string          `json:"cwe,omitempty"`
	CVE         []string          `json:"cve,omitempty"`
	Location    string            `json:"location"`
	// Remediation is the rule's own deterministic guidance
	// (domain.Finding.Remediation) — never AI-generated, per the
	// project-wide rule that remediation must stand alone without AI
	// (CLAUDE.md's security posture section, documentation/05-module-specifications.md
	// §2.3). The report's one AI call is spent on ExecutiveSummary instead
	// of one call per finding, which is what "don't waste Gemini's API" —
	// requires: rule-authored remediation is already free and already
	// required on every finding, so spending budget to paraphrase it would
	// buy nothing.
	Remediation string `json:"remediation"`
}
