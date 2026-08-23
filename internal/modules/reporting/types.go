// Package reporting assembles a completed scan into a self-contained report
// and renders it as JSON, CSV, or PDF (documentation/07-api-specification.md
// §5's `GET /scans/{id}/export`, BUILD_GUIDE.md Phase 13 — export pulled
// forward ahead of scoring/triage/correlation, which this package does not
// touch). One Assembler.Build call produces one ReportData; every format
// renders from that same struct, so the three formats can never disagree
// about what a report says.
//
// modules/scoring doesn't exist yet, so ReportData carries no risk score —
// a report is honest about what's actually computed today (finding counts,
// engine coverage, an AI-authored narrative) rather than fabricating a
// number nothing has calculated.
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
	TotalScriptRuns     int      `json:"total_script_runs"`
	PhasesCompleted     []string `json:"phases_completed"`
	PhasesSkipped       []string `json:"phases_skipped,omitempty"`
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
