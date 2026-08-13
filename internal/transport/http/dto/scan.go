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
	ID               string        `json:"id"`
	ProjectID        string        `json:"project_id"`
	Type             string        `json:"type"`
	Status           string        `json:"status"`
	RequestedEngines []string      `json:"requested_engines"`
	Branch           *string       `json:"branch"`
	CommitSHA        *string       `json:"commit_sha"`
	QueuedAt         time.Time     `json:"queued_at"`
	StartedAt        *time.Time    `json:"started_at"`
	FinishedAt       *time.Time    `json:"finished_at"`
	FindingCounts    map[string]int `json:"finding_counts"`
	Risk             any           `json:"risk"`
	Jobs             []JobResponse `json:"jobs"`
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
	ScanID      string                    `json:"scan_id"`
	Status      string                    `json:"status"`
	ProgressPct int                       `json:"progress_pct"`
	Engines     []EngineProgressResponse  `json:"engines"`
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

// FindingListItemResponse matches `GET /scans/{id}/findings`'s list item —
// deliberately lighter than a full detail view (documentation/07-api-specification.md
// §6: "the list renders 1,000 rows"). first_seen_at/age_days/has_ai_suggestion
// are Phase 13's (cross-scan history, AI) — omitted rather than faked.
type FindingListItemResponse struct {
	ID          string            `json:"id"`
	Engine      string            `json:"engine"`
	RuleID      string            `json:"rule_id"`
	Title       string            `json:"title"`
	Severity    string            `json:"severity"`
	Confidence  string            `json:"confidence"`
	Status      string            `json:"status"`
	CWE         []string          `json:"cwe"`
	CVE         []string          `json:"cve"`
	OWASP       []string          `json:"owasp"`
	CVSSScore   *float64          `json:"cvss_score"`
	Location    LocationResponse  `json:"location"`
}

func FromFinding(f domain.Finding) FindingListItemResponse {
	return FindingListItemResponse{
		ID: f.ID.String(), Engine: string(f.Engine), RuleID: f.RuleID, Title: f.Title,
		Severity: string(f.Severity), Confidence: string(f.Confidence), Status: string(f.Status),
		CWE: emptyIfNilStrings(f.CWE), CVE: emptyIfNilStrings(f.CVE), OWASP: emptyIfNilStrings(f.OWASP),
		CVSSScore: f.CVSSScore, Location: fromLocation(f.Location),
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
