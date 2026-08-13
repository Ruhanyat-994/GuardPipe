package dto

import (
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
)

// --- scans — documentation/07-api-specification.md §5 ---

// CreateScanRequest matches `POST /projects/{id}/scans`. Engines is only
// used when Type is "partial".
type CreateScanRequest struct {
	Type    string   `json:"type" validate:"required,oneof=full_supply_chain partial pentest_only"`
	Engines []string `json:"engines" validate:"omitempty,dive,oneof=docreview codescan depscan containerscan k8sscan cicdscan pentest"`
	Branch  string   `json:"branch"`
}

func (r CreateScanRequest) ToInput() orchestrator.CreateScanInput {
	engines := make([]domain.EngineID, len(r.Engines))
	for i, e := range r.Engines {
		engines[i] = domain.EngineID(e)
	}
	return orchestrator.CreateScanInput{Type: domain.ScanType(r.Type), Engines: engines, Branch: r.Branch}
}

// JobResponse is one entry in ScanResponse.Jobs.
type JobResponse struct {
	ID           string     `json:"id"`
	Engine       string     `json:"engine"`
	Status       string     `json:"status"`
	StartedAt    *time.Time `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
	FindingCount int        `json:"finding_count"`
	Stats        any        `json:"stats"`
	ErrorReason  *string    `json:"error_reason"`
	SkipReason   *string    `json:"skip_reason"`
}

func fromJobDetail(j orchestrator.JobDetail) JobResponse {
	return JobResponse{
		ID: j.ID.String(), Engine: string(j.Engine), Status: string(j.Status),
		StartedAt: j.StartedAt, FinishedAt: j.FinishedAt, FindingCount: j.FindingCount,
		Stats: j.Stats, ErrorReason: j.ErrorReason, SkipReason: j.SkipReason,
	}
}

// ScanResponse matches `POST /projects/{id}/scans`'s 202 body and
// `GET /scans/{id}`'s 200 body — the same shape serves both.
// Risk/AI-derived fields are Phase 13's; per the "nulls are present, not
// omitted" convention (documentation/07-api-specification.md §1), risk is
// always present as null until that phase builds it, not silently dropped.
type ScanResponse struct {
	ID               string         `json:"id"`
	ProjectID        string         `json:"project_id"`
	Type             string         `json:"type"`
	Status           string         `json:"status"`
	RequestedEngines []string       `json:"requested_engines"`
	Branch           *string        `json:"branch"`
	CommitSHA        *string        `json:"commit_sha"`
	QueuedAt         time.Time      `json:"queued_at"`
	StartedAt        *time.Time     `json:"started_at"`
	FinishedAt       *time.Time     `json:"finished_at"`
	FindingCounts    map[string]int `json:"finding_counts"`
	Risk             any            `json:"risk"`
	Jobs             []JobResponse  `json:"jobs"`
}

func FromScanDetail(d *orchestrator.ScanDetail) ScanResponse {
	engines := make([]string, len(d.RequestedEngines))
	for i, e := range d.RequestedEngines {
		engines[i] = string(e)
	}
	counts := make(map[string]int, len(d.FindingCounts))
	for k, v := range d.FindingCounts {
		counts[string(k)] = v
	}
	jobs := make([]JobResponse, len(d.Jobs))
	for i, j := range d.Jobs {
		jobs[i] = fromJobDetail(j)
	}

	return ScanResponse{
		ID: d.ID.String(), ProjectID: d.ProjectID.String(), Type: string(d.Type), Status: string(d.Status),
		RequestedEngines: engines, Branch: d.Branch, CommitSHA: d.CommitSHA,
		QueuedAt: d.QueuedAt, StartedAt: d.StartedAt, FinishedAt: d.FinishedAt,
		FindingCounts: counts, Risk: nil, Jobs: jobs,
	}
}

// ScanSummaryResponse is one row of `GET /projects/{id}/scans`'s scan
// history list — deliberately lighter than ScanResponse (no per-job detail,
// which would mean an N+1 job/finding-count query per row for a list
// endpoint) but still carries everything the history table needs to render
// date/time, type, status, and finding counts without a follow-up request.
type ScanSummaryResponse struct {
	ID            string         `json:"id"`
	ProjectID     string         `json:"project_id"`
	Type          string         `json:"type"`
	Status        string         `json:"status"`
	Branch        *string        `json:"branch"`
	QueuedAt      time.Time      `json:"queued_at"`
	StartedAt     *time.Time     `json:"started_at"`
	FinishedAt    *time.Time     `json:"finished_at"`
	FindingCounts map[string]int `json:"finding_counts"`
}

func FromScan(s domain.Scan) ScanSummaryResponse {
	counts := make(map[string]int, len(s.FindingCounts))
	for k, v := range s.FindingCounts {
		counts[string(k)] = v
	}
	return ScanSummaryResponse{
		ID: s.ID.String(), ProjectID: s.ProjectID.String(), Type: string(s.Type), Status: string(s.Status),
		Branch: s.Branch, QueuedAt: s.QueuedAt, StartedAt: s.StartedAt, FinishedAt: s.FinishedAt,
		FindingCounts: counts,
	}
}

// ScanListResponse matches `GET /projects/{id}/scans`.
type ScanListResponse struct {
	Data       []ScanSummaryResponse `json:"data"`
	Pagination Pagination            `json:"pagination"`
}

// OrgScanSummaryResponse is one row of `GET /scans`'s org-wide scan
// history — ScanSummaryResponse plus the project name, so the global
// Scans page can render "Project · Scan" without a second request per row.
type OrgScanSummaryResponse struct {
	ScanSummaryResponse
	ProjectName string `json:"project_name"`
}

func FromOrgScanSummary(s orchestrator.OrgScanSummary) OrgScanSummaryResponse {
	return OrgScanSummaryResponse{ScanSummaryResponse: FromScan(s.Scan), ProjectName: s.ProjectName}
}

// OrgScanListResponse matches `GET /scans`.
type OrgScanListResponse struct {
	Data       []OrgScanSummaryResponse `json:"data"`
	Pagination Pagination                `json:"pagination"`
}

// EngineProgressResponse is one entry in ProgressResponse.Engines.
type EngineProgressResponse struct {
	Engine       string `json:"engine"`
	Status       string `json:"status"`
	ProgressPct  int    `json:"progress_pct"`
	FindingCount int    `json:"finding_count"`
}

// ProgressResponse matches `GET /scans/{id}/progress` — deliberately tiny
// (documentation/07-api-specification.md §5).
type ProgressResponse struct {
	ScanID      string                   `json:"scan_id"`
	Status      string                   `json:"status"`
	ProgressPct int                      `json:"progress_pct"`
	Engines     []EngineProgressResponse `json:"engines"`
}

func FromProgress(p *orchestrator.Progress) ProgressResponse {
	engines := make([]EngineProgressResponse, len(p.Engines))
	for i, e := range p.Engines {
		engines[i] = EngineProgressResponse{
			Engine: string(e.Engine), Status: string(e.Status),
			ProgressPct: e.ProgressPct, FindingCount: e.FindingCount,
		}
	}
	return ProgressResponse{ScanID: p.ScanID.String(), Status: string(p.Status), ProgressPct: p.ProgressPct, Engines: engines}
}

// --- findings — documentation/07-api-specification.md §6, list only
// (detail/triage/history/explain are Phase 13's) ---

// LocationResponse mirrors domain.Location's discriminated union directly
// — the frontend already switches on `type` per documentation/06-database-design.md
// §5, so this is a pass-through, not a re-shape.
type LocationResponse struct {
	Type         string `json:"type"`
	Path         string `json:"path,omitempty"`
	LineStart    int    `json:"line_start,omitempty"`
	LineEnd      int    `json:"line_end,omitempty"`
	Column       int    `json:"column,omitempty"`
	Image        string `json:"image,omitempty"`
	LayerDigest  string `json:"layer_digest,omitempty"`
	LayerIndex   int    `json:"layer_index,omitempty"`
	File         string `json:"file,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Name         string `json:"name,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
	FieldPath    string `json:"field_path,omitempty"`
	Host         string `json:"host,omitempty"`
	IP           string `json:"ip,omitempty"`
	Port         int    `json:"port,omitempty"`
	Protocol     string `json:"protocol,omitempty"`
	Service      string `json:"service,omitempty"`
	URL          string `json:"url,omitempty"`
	Ecosystem    string `json:"ecosystem,omitempty"`
	Package      string `json:"package,omitempty"`
	Version      string `json:"version,omitempty"`
	ManifestPath string `json:"manifest_path,omitempty"`
}

func fromLocation(l domain.Location) LocationResponse {
	return LocationResponse{
		Type: string(l.Type), Path: l.Path, LineStart: l.LineStart, LineEnd: l.LineEnd, Column: l.Column,
		Image: l.Image, LayerDigest: l.LayerDigest, LayerIndex: l.LayerIndex,
		File: l.File, Kind: l.Kind, Name: l.Name, Namespace: l.Namespace, FieldPath: l.FieldPath,
		Host: l.Host, IP: l.IP, Port: l.Port, Protocol: l.Protocol, Service: l.Service, URL: l.URL,
		Ecosystem: l.Ecosystem, Package: l.Package, Version: l.Version, ManifestPath: l.ManifestPath,
	}
}

// EvidenceResponse is one supporting item behind a finding's "More" view —
// a redacted code/manifest excerpt or command transcript, plus the exact
// lines it came from. Value is always safe to render as-is: engines redact
// anything secret-shaped before a Finding is ever emitted
// (documentation/06-database-design.md §4.12), this is a pass-through of
// already-redacted content, never the raw match.
type EvidenceResponse struct {
	Kind      string `json:"kind"`
	Value     string `json:"value"`
	Redacted  bool   `json:"redacted"`
	LineStart int    `json:"line_start,omitempty"`
	LineEnd   int    `json:"line_end,omitempty"`
}

func fromEvidence(e domain.Evidence) EvidenceResponse {
	return EvidenceResponse{Kind: string(e.Kind), Value: e.Value, Redacted: e.Redacted, LineStart: e.LineStart, LineEnd: e.LineEnd}
}

// FindingListItemResponse matches `GET /scans/{id}/findings`'s list item.
// Description/Remediation/Evidence make each row self-sufficient for a
// "More" detail expansion without a second request — the list is capped at
// 100 rows a page, so carrying this here is cheap and avoids a per-finding
// detail endpoint that doesn't otherwise exist yet. Remediation is
// deterministic guidance straight from the rule (documentation/03-architecture-overview.md
// §7.1: "must stand alone without AI"); an AI-authored explanation/patch is
// a separate, later field (Phase 10/11), not a replacement for this one.
// first_seen_at/age_days are Phase 13's (cross-scan history) — omitted
// rather than faked.
type FindingListItemResponse struct {
	ID          string             `json:"id"`
	Engine      string             `json:"engine"`
	RuleID      string             `json:"rule_id"`
	Title       string             `json:"title"`
	Description string             `json:"description"`
	Remediation string             `json:"remediation"`
	Severity    string             `json:"severity"`
	Confidence  string             `json:"confidence"`
	Status      string             `json:"status"`
	CWE         []string           `json:"cwe"`
	CVE         []string           `json:"cve"`
	OWASP       []string           `json:"owasp"`
	CVSSScore   *float64           `json:"cvss_score"`
	Location    LocationResponse   `json:"location"`
	Evidence    []EvidenceResponse `json:"evidence"`
}

func FromFinding(f domain.Finding) FindingListItemResponse {
	evidence := make([]EvidenceResponse, len(f.Evidence))
	for i, e := range f.Evidence {
		evidence[i] = fromEvidence(e)
	}
	return FindingListItemResponse{
		ID: f.ID.String(), Engine: string(f.Engine), RuleID: f.RuleID, Title: f.Title,
		Description: f.Description, Remediation: f.Remediation,
		Severity: string(f.Severity), Confidence: string(f.Confidence), Status: string(f.Status),
		CWE: emptyIfNilStrings(f.CWE), CVE: emptyIfNilStrings(f.CVE), OWASP: emptyIfNilStrings(f.OWASP),
		CVSSScore: f.CVSSScore, Location: fromLocation(f.Location), Evidence: evidence,
	}
}

func emptyIfNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// FindingListResponse matches `GET /scans/{id}/findings`.
type FindingListResponse struct {
	Data       []FindingListItemResponse `json:"data"`
	Pagination Pagination                `json:"pagination"`
}
