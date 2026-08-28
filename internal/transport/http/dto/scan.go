package dto

import (
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
)

// --- scans — documentation/07-api-specification.md §5 ---

// CreateScanRequest matches `POST /projects/{id}/scans`. Engines is only
// used when Type is "partial". PentestConfig is optional — omitted means
// "use the server-side default" (Stealth, literally — BUILD_GUIDE.md Phase
// 12's "default-to-Stealth is literal, not just a UI default" rule);
// ignored when the resolved engine set doesn't include pentest.
type CreateScanRequest struct {
	Type          string                `json:"type" validate:"required,oneof=full_supply_chain partial pentest_only"`
	Engines       []string              `json:"engines" validate:"omitempty,dive,oneof=docreview codescan depscan containerscan k8sscan cicdscan pentest"`
	Branch        string                `json:"branch"`
	PentestConfig *PentestConfigRequest `json:"pentest_config,omitempty"`
}

func (r CreateScanRequest) ToInput(sourceIP string) orchestrator.CreateScanInput {
	engines := make([]domain.EngineID, len(r.Engines))
	for i, e := range r.Engines {
		engines[i] = domain.EngineID(e)
	}
	in := orchestrator.CreateScanInput{Type: domain.ScanType(r.Type), Engines: engines, Branch: r.Branch, SourceIP: sourceIP}
	if r.PentestConfig != nil {
		cfg := r.PentestConfig.toDomain()
		in.PentestConfig = &cfg
	}
	return in
}

// PentestConfigRequest is BUILD_GUIDE.md Phase 12's "client-controllable
// scan intensity" request shape — a named preset, optionally with one or
// more fields overridden (which switches the effective preset to "custom",
// same rule domain.ClampPentestScanConfig itself enforces server-side).
// Preset alone (no overrides) is the common case: pick a card, done.
type PentestConfigRequest struct {
	Preset             string   `json:"preset" validate:"omitempty,oneof=stealth standard deep custom"`
	RequestRatePerSec  int      `json:"request_rate_per_sec,omitempty"`
	PortBreadth        string   `json:"port_breadth,omitempty" validate:"omitempty,oneof=top100 top1000 top1000_service_detect"`
	NucleiCategories   []string `json:"nuclei_categories,omitempty"`
	PhaseBudgetSeconds int      `json:"phase_budget_seconds,omitempty"`
	// WordlistTier ("small"/"medium"/"large") sizes the subdomain-enum
	// phase's active DNS brute-force wordlist (and the disclosure phase's
	// dynamic-wordlist cap) — omitted means "use the preset's own tier."
	WordlistTier string `json:"wordlist_tier,omitempty" validate:"omitempty,oneof=small medium large"`
	// SubdomainEnum toggles the whole subdomain/asset-enumeration phase —
	// a pointer so "omitted" (use the preset's own default, true) is
	// distinguishable from an explicit `false` override.
	SubdomainEnum *bool `json:"subdomain_enum,omitempty"`
}

func (r *PentestConfigRequest) toDomain() domain.PentestScanConfig {
	base := domain.PentestPresetStealthConfig()
	switch domain.PentestPreset(r.Preset) {
	case domain.PentestPresetStandard:
		base = domain.PentestPresetStandardConfig()
	case domain.PentestPresetDeep:
		base = domain.PentestPresetDeepConfig()
	}

	if r.RequestRatePerSec > 0 {
		base.RequestRatePerSec = r.RequestRatePerSec
		base.Preset = domain.PentestPresetCustom
	}
	if r.PortBreadth != "" {
		base.PortBreadth = domain.PentestPortBreadth(r.PortBreadth)
		base.Preset = domain.PentestPresetCustom
	}
	if len(r.NucleiCategories) > 0 {
		base.NucleiCategories = r.NucleiCategories
		base.Preset = domain.PentestPresetCustom
	}
	if r.PhaseBudgetSeconds > 0 {
		base.PhaseBudget = time.Duration(r.PhaseBudgetSeconds) * time.Second
		base.Preset = domain.PentestPresetCustom
	}
	if r.WordlistTier != "" {
		base.WordlistTier = r.WordlistTier
		base.Preset = domain.PentestPresetCustom
	}
	if r.SubdomainEnum != nil {
		base.SubdomainEnum = *r.SubdomainEnum
		base.Preset = domain.PentestPresetCustom
	}
	return base
}

// PentestConfigResponse mirrors PentestConfigRequest's shape back —
// ScanResponse's copy is always the resolved, already-clamped-to-the-
// ceiling config a scan actually ran (or will run) with, never the raw
// client request. ClampedFields is populated only on the response to the
// CreateScan call itself (BUILD_GUIDE.md Phase 12: "scan speed capped at 10
// req/s"); a later GET /scans/{id} carries the resolved config with an
// empty ClampedFields, since nothing was clamped *this read*.
type PentestConfigResponse struct {
	Preset             string   `json:"preset"`
	RequestRatePerSec  int      `json:"request_rate_per_sec"`
	PortBreadth        string   `json:"port_breadth"`
	NucleiCategories   []string `json:"nuclei_categories"`
	PhaseBudgetSeconds int      `json:"phase_budget_seconds"`
	WordlistTier       string   `json:"wordlist_tier"`
	SubdomainEnum      bool     `json:"subdomain_enum"`
	ClampedFields      []string `json:"clamped_fields,omitempty"`
}

func fromPentestConfig(cfg *domain.PentestScanConfig, clampedFields []string) *PentestConfigResponse {
	if cfg == nil {
		return nil
	}
	return &PentestConfigResponse{
		Preset: string(cfg.Preset), RequestRatePerSec: cfg.RequestRatePerSec,
		PortBreadth: string(cfg.PortBreadth), NucleiCategories: cfg.NucleiCategories,
		PhaseBudgetSeconds: int(cfg.PhaseBudget / time.Second),
		WordlistTier:       cfg.WordlistTier, SubdomainEnum: cfg.SubdomainEnum,
		ClampedFields: clampedFields,
	}
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

// RiskAssessmentResponse matches `GET /scans/{id}`'s "risk" field
// (documentation/07-api-specification.md §5). nil (the whole
// ScanResponse.Risk field, not this type) until the scan's last job
// finalizes and Pool.finalizeScoring (worker.go) persists one.
type RiskAssessmentResponse struct {
	Score         int    `json:"score"`
	Verdict       string `json:"verdict"`
	PreviousScore *int   `json:"previous_score"`
	// Delta is Score - PreviousScore: negative means improving, positive
	// means regressing (documentation/11-risk-scoring-and-severity.md §6).
	// nil whenever PreviousScore is nil.
	Delta        *int           `json:"delta"`
	IsPartial    bool           `json:"is_partial"`
	EngineScores map[string]int `json:"engine_scores"`
	// Breakdown is the ordered, human-readable "why is the score what it
	// is" explanation a RiskGauge renders underneath itself
	// (documentation/11-risk-scoring-and-severity.md §4's worked example;
	// requirement 2, "the UI shows the breakdown"). Doc 07's own §5 example
	// JSON omits this field, but every other field in that example is
	// abbreviated too (e.g. jobs shows only 2 of what a real scan has) —
	// treated as an omission for brevity, not a deliberate exclusion.
	Breakdown      []ContributionResponse `json:"breakdown"`
	FormulaVersion string                 `json:"formula_version"`
}

// ContributionResponse is one entry in RiskAssessmentResponse.Breakdown.
type ContributionResponse struct {
	Reason string  `json:"reason"`
	Engine string  `json:"engine,omitempty"`
	Detail string  `json:"detail"`
	Impact float64 `json:"impact"`
}

func fromRiskAssessment(r *orchestrator.RiskAssessmentRecord) *RiskAssessmentResponse {
	if r == nil {
		return nil
	}
	engineScores := make(map[string]int, len(r.EngineScores))
	for e, s := range r.EngineScores {
		engineScores[string(e)] = s
	}
	breakdown := make([]ContributionResponse, len(r.Breakdown))
	for i, c := range r.Breakdown {
		breakdown[i] = ContributionResponse{Reason: string(c.Reason), Engine: string(c.Engine), Detail: c.Detail, Impact: c.Impact}
	}
	return &RiskAssessmentResponse{
		Score: r.Score, Verdict: string(r.Verdict), PreviousScore: r.PreviousScore, Delta: r.Delta,
		IsPartial: r.IsPartial, EngineScores: engineScores, Breakdown: breakdown, FormulaVersion: r.FormulaVersion,
	}
}

// ScanResponse matches `POST /projects/{id}/scans`'s 202 body and
// `GET /scans/{id}`'s 200 body — the same shape serves both.
// Risk is nil (not omitted — documentation/07-api-specification.md §1's
// "nulls are present, not omitted" convention) until the scan's last job
// finalizes and a real score exists to show.
type ScanResponse struct {
	ID               string                  `json:"id"`
	ProjectID        string                  `json:"project_id"`
	Type             string                  `json:"type"`
	Status           string                  `json:"status"`
	RequestedEngines []string                `json:"requested_engines"`
	Branch           *string                 `json:"branch"`
	CommitSHA        *string                 `json:"commit_sha"`
	QueuedAt         time.Time               `json:"queued_at"`
	StartedAt        *time.Time              `json:"started_at"`
	FinishedAt       *time.Time              `json:"finished_at"`
	FindingCounts    map[string]int          `json:"finding_counts"`
	Risk             *RiskAssessmentResponse `json:"risk"`
	Jobs             []JobResponse           `json:"jobs"`
	// ScanNumber is this scan's 1-based position among its own project's
	// scans (oldest = 1) — the UI's "Scan #N" in place of a raw UUID prefix.
	ScanNumber int `json:"scan_number"`
	// PentestConfig is nil for a scan with no pentest job. BUILD_GUIDE.md
	// Phase 12: "whatever preset/custom config was actually used is shown
	// on the completed scan's detail view."
	PentestConfig *PentestConfigResponse `json:"pentest_config"`
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
		FindingCounts: counts, Risk: fromRiskAssessment(d.Risk), Jobs: jobs, ScanNumber: d.ScanNumber,
		PentestConfig: fromPentestConfig(d.PentestConfig, d.PentestConfigClamped),
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
	// ScanNumber is this scan's 1-based position among its own project's
	// scans (oldest = 1) — the UI's "Scan #N" in place of a raw UUID prefix.
	ScanNumber int `json:"scan_number"`
}

func FromScan(s domain.Scan) ScanSummaryResponse {
	counts := make(map[string]int, len(s.FindingCounts))
	for k, v := range s.FindingCounts {
		counts[string(k)] = v
	}
	return ScanSummaryResponse{
		ID: s.ID.String(), ProjectID: s.ProjectID.String(), Type: string(s.Type), Status: string(s.Status),
		Branch: s.Branch, QueuedAt: s.QueuedAt, StartedAt: s.StartedAt, FinishedAt: s.FinishedAt,
		FindingCounts: counts, ScanNumber: s.ScanNumber,
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
	Pagination Pagination               `json:"pagination"`
}

// EngineProgressResponse is one entry in ProgressResponse.Engines.
type EngineProgressResponse struct {
	Engine      string `json:"engine"`
	Status      string `json:"status"`
	ProgressPct int    `json:"progress_pct"`
	// Activity is a real, live "what's happening right now" label — an
	// engine's own reported stage when it has one (pentest's actual phase
	// names), omitted otherwise. Absence is normal, not an error: the
	// frontend already has a generic per-engine fallback label for engines
	// that don't report named stages.
	Activity     string `json:"activity,omitempty"`
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
			ProgressPct: e.ProgressPct, Activity: e.Activity, FindingCount: e.FindingCount,
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
	Container    string `json:"container,omitempty"`
	FieldPath    string `json:"field_path,omitempty"`
	Value        string `json:"value,omitempty"`
	FromHelm     bool   `json:"from_helm,omitempty"`
	ChartName    string `json:"chart_name,omitempty"`
	TemplateFile string `json:"template_file,omitempty"`
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
		File: l.File, Kind: l.Kind, Name: l.Name, Namespace: l.Namespace, Container: l.Container,
		FieldPath: l.FieldPath, Value: l.Value,
		FromHelm: l.FromHelm, ChartName: l.ChartName, TemplateFile: l.TemplateFile,
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
	CVSSVector  *string            `json:"cvss_vector"`
	Location    LocationResponse   `json:"location"`
	Evidence    []EvidenceResponse `json:"evidence"`
	// Source is "rule" or "ai" — cicdscan (Phase 10) is the first engine
	// whose findings can be AI-authored (documentation/05-module-specifications.md
	// §10's semantic pass); the frontend uses this to render the
	// "AI-generated" chip. Always "rule" for every earlier engine.
	Source string `json:"source"`
	// Metadata is engine-specific extra context — never a load-bearing
	// field for any core behaviour (documentation/06-database-design.md's
	// own "JSONB only for genuinely variable data" rule), just narrative
	// detail: e.g. k8sscan/containerscan's "impact"/"attack_path" (a short
	// why-this-matters sentence and an ordered escalation-chain array, see
	// each engine's own attackpath.go), or depscan/containerscan's
	// "osv_id"/"trivy_check_id" upstream references. Absent keys are
	// absent, never faked with empty strings.
	Metadata map[string]any `json:"metadata,omitempty"`
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
		CVSSScore: f.CVSSScore, CVSSVector: f.CVSSVector, Location: fromLocation(f.Location), Evidence: evidence,
		Metadata: f.Metadata, Source: string(f.Source.Effective()),
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
