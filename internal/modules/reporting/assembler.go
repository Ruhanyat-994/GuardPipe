package reporting

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
)

// responsibilityDisclaimer is every report's fixed accountability statement
// — modelled on the same "the tool doesn't authorise anything, the operator
// does" posture Nessus/Qualys/Burp Suite-class scanners already put on
// their own output, since GuardPipe's target-admission model (denylist, not
// allowlist — CLAUDE.md's security-posture section) accepts any
// publicly-resolvable host by design and relies on the attestation, not a
// pre-approval list, as the real authorisation control. Not legal advice —
// review with counsel before this is a customer-facing ToS.
const responsibilityDisclaimer = "This scan was initiated by the account identified above, who attested ownership of or explicit authorisation to test the scanned target and is solely responsible for ensuring that authorisation is genuine, current, and sufficient in scope. GuardPipe is a testing tool; it does not verify real-world authorisation on the requester's behalf and accepts no liability for scans run without it. Every scan is logged against the initiating account and source IP address."

// ScanReader is the subset of orchestrator.Service Assembler needs — defined
// here (the consumer), same convention internal/modules/orchestrator/worker.go's
// Cloner already uses, so a test substitutes a small hand-written fake
// instead of implementing orchestrator.Service's full surface.
type ScanReader interface {
	GetScan(ctx context.Context, actor domain.Actor, scanID uuid.UUID) (*orchestrator.ScanDetail, error)
	ListFindings(ctx context.Context, actor domain.Actor, scanID uuid.UUID, page orchestrator.Page) ([]domain.Finding, int, error)
}

// ProjectReader is the subset of project.Service Assembler needs — same
// narrow-consumer-interface convention as ScanReader above.
type ProjectReader interface {
	Get(ctx context.Context, actor domain.Actor, id uuid.UUID) (*project.ProjectDetail, error)
}

// UserReader resolves a scan's triggering user for the report's
// accountability stamp — implemented directly by *store/repo.UserRepo
// (already exposes exactly this method for identity's own auth flows), no
// new repository code needed. A lookup failure (e.g. the user was since
// deleted) is tolerated, not fatal to Build — see attachRequestedBy.
type UserReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*identity.User, error)
}

// findingsPageSize is how many findings Assembler.Build asks
// orchestrator.Service.ListFindings for per call — the service's own
// documented page-size ceiling (documentation/07-api-specification.md
// §6, max 100) is respected by looping rather than requesting one huge
// page.
const findingsPageSize = 100

// topPrioritiesCount is how many of the worst findings feed the AI
// executive-summary prompt's {{.top_findings}} var — enough for a genuinely
// useful summary without inflating the one call this package ever makes to
// Gemini per report.
const topPrioritiesCount = 5

// Assembler builds a ReportData from a completed scan, calling only
// orchestrator.Service and project.Service's already-published, already
// actor-authorized interfaces (documentation/03-architecture-overview.md
// §6.2's dependency rule) — no direct table access of its own.
type Assembler struct {
	scans    ScanReader
	projects ProjectReader
	users    UserReader
	ai       ai.Service // nil disables the AI executive summary entirely
	log      *slog.Logger
}

// NewAssembler wires an Assembler. aiSvc may be nil (AI disabled or no
// Gemini key configured — same convention every AI-consuming engine already
// follows) — Build still succeeds, just without ExecutiveSummary/TopPriorities.
// users may also be nil (a test that doesn't care about the accountability
// stamp) — Build then just leaves RequestedByName/Email empty.
func NewAssembler(scans ScanReader, projects ProjectReader, users UserReader, aiSvc ai.Service, log *slog.Logger) *Assembler {
	if log == nil {
		log = slog.Default()
	}
	return &Assembler{scans: scans, projects: projects, users: users, ai: aiSvc, log: log}
}

// Build assembles one report. actor's org must own the scan (both
// GetScan/ListFindings enforce that themselves — a cross-org request gets
// the same 404 every other scan endpoint returns, not a reporting-specific
// authorization path).
func (a *Assembler) Build(ctx context.Context, actor domain.Actor, scanID uuid.UUID) (*ReportData, error) {
	detail, err := a.scans.GetScan(ctx, actor, scanID)
	if err != nil {
		return nil, err
	}
	proj, err := a.projects.Get(ctx, actor, detail.ProjectID)
	if err != nil {
		return nil, err
	}
	findings, err := a.fetchAllFindings(ctx, actor, scanID)
	if err != nil {
		return nil, err
	}

	data := &ReportData{
		GeneratedAt:     time.Now().UTC(),
		ProjectName:     proj.Name,
		ScanID:          detail.ID,
		ScanNumber:      detail.ScanNumber,
		ScanType:        detail.Type,
		ScanStatus:      detail.Status,
		QueuedAt:        detail.QueuedAt,
		StartedAt:       detail.StartedAt,
		FinishedAt:      detail.FinishedAt,
		FindingCounts:   detail.FindingCounts,
		RequestedFromIP: detail.RequestedFromIP,
		Disclaimer:      responsibilityDisclaimer,
	}
	a.attachRequestedBy(ctx, data, detail.TriggeredBy)
	if detail.CommitSHA != nil {
		data.CommitSHA = *detail.CommitSHA
	}
	if detail.Branch != nil {
		data.Branch = *detail.Branch
	}
	if proj.Repository != nil {
		data.RepositoryOwner = proj.Repository.Owner
		data.RepositoryName = proj.Repository.Name
		data.RepositoryBranch = proj.Repository.DefaultBranch
	}

	data.Jobs = make([]JobSummary, 0, len(detail.Jobs))
	for _, j := range detail.Jobs {
		js := JobSummary{
			Engine: j.Engine, Status: j.Status,
			StartedAt: j.StartedAt, FinishedAt: j.FinishedAt,
			FindingCount: j.FindingCount,
		}
		if j.ErrorReason != nil {
			js.ErrorReason = *j.ErrorReason
		}
		if j.SkipReason != nil {
			js.SkipReason = *j.SkipReason
		}
		if j.Engine == domain.EnginePentest && j.Stats != nil {
			if cov, ok := decodeCoverage(j.Stats); ok {
				js.Coverage = cov
			}
			if host := stringStat(j.Stats, "target_host"); host != "" {
				data.PentestTargetHost = host
			}
			js.ValidatedSubdomains = decodeValidatedSubdomains(j.Stats)
			js.ValidatedDirectories = decodeValidatedDirectories(j.Stats)
		}
		data.Jobs = append(data.Jobs, js)
	}

	data.Findings = make([]FindingRow, 0, len(findings))
	for _, f := range findings {
		data.Findings = append(data.Findings, FindingRow{
			Engine: f.Engine, RuleID: f.RuleID, Title: f.Title, Description: f.Description,
			Severity: f.Severity, Confidence: f.Confidence, CWE: f.CWE, CVE: f.CVE,
			Location: FormatLocation(f.Location), Remediation: f.Remediation,
		})
	}
	sort.SliceStable(data.Findings, func(i, j int) bool {
		return data.Findings[i].Severity.Rank() < data.Findings[j].Severity.Rank()
	})
	data.TotalFindings = len(data.Findings)

	a.attachExecutiveSummary(ctx, data, detail.Risk)

	return data, nil
}

// attachRequestedBy resolves triggeredBy to the display name/email the
// report's accountability stamp shows — tolerant of every failure mode
// (nil users reader, no triggering user recorded, the user was since
// deleted): Build must still succeed and produce a complete report, just
// with RequestedByName/Email left empty, exactly the same fallback contract
// attachExecutiveSummary already follows for a missing/failed AI call.
func (a *Assembler) attachRequestedBy(ctx context.Context, data *ReportData, triggeredBy *uuid.UUID) {
	if a.users == nil || triggeredBy == nil {
		return
	}
	u, err := a.users.GetByID(ctx, *triggeredBy)
	if err != nil {
		a.log.Warn("reporting: could not resolve requesting user for report watermark", "scan_id", data.ScanID, "user_id", *triggeredBy, "error", err)
		return
	}
	data.RequestedByName = u.DisplayName
	data.RequestedByEmail = u.Email
}

// fetchAllFindings pages through orchestrator.Service.ListFindings until
// every finding for the scan is collected — a report has to be complete, so
// this can't stop at one page the way a paginated UI list does.
func (a *Assembler) fetchAllFindings(ctx context.Context, actor domain.Actor, scanID uuid.UUID) ([]domain.Finding, error) {
	var all []domain.Finding
	for page := 1; ; page++ {
		batch, total, err := a.scans.ListFindings(ctx, actor, scanID, orchestrator.Page{Page: page, PageSize: findingsPageSize})
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) == 0 || len(all) >= total {
			break
		}
	}
	return all, nil
}

// attachExecutiveSummary makes exactly one AI call per Build — the report's
// entire Gemini budget — using the already-registered, already-versioned
// ai.PromptSummariseScan prompt (internal/modules/ai/prompts.go), content-
// hash cached like every other ai.Service call, so re-downloading the same
// scan's report doesn't spend a second call. Never fails Build: any error,
// discard, or a nil ai.Service just leaves ExecutiveSummary/TopPriorities
// empty, the same fallback contract every AI-consuming engine already
// follows (documentation/10-ai-integration.md §9). risk is detail.Risk from
// Build's own GetScan call — nil for a scan whose last job hasn't finalized
// yet (Pool.finalizeScoring, worker.go, hasn't run), in which case the
// prompt is told exactly that rather than a fabricated number.
func (a *Assembler) attachExecutiveSummary(ctx context.Context, data *ReportData, risk *orchestrator.RiskAssessmentRecord) {
	if a.ai == nil {
		return
	}
	riskScore, verdict := "not yet computed for this scan", "not yet computed"
	if risk != nil {
		riskScore = fmt.Sprintf("%d", risk.Score)
		verdict = string(risk.Verdict)
	}
	vars := map[string]string{
		"risk_score":      riskScore,
		"verdict":         verdict,
		"finding_counts":  formatFindingCounts(data.FindingCounts),
		"top_findings":    formatTopFindings(data.Findings, topPrioritiesCount),
		"engine_statuses": formatEngineStatuses(data.Jobs),
	}
	result, err := a.ai.Run(ctx, ai.RunInput{PromptID: ai.PromptSummariseScan, Vars: vars, ScanID: data.ScanID}, nil)
	if err != nil {
		a.log.Warn("reporting: AI executive summary unavailable, report will omit it", "scan_id", data.ScanID, "error", err)
		return
	}
	if result.Discarded {
		a.log.Warn("reporting: AI executive summary discarded (suspected prompt injection), report will omit it", "scan_id", data.ScanID)
		return
	}
	summary, ok := result.Value.(ai.ScanSummaryResponse)
	if !ok {
		a.log.Warn("reporting: AI executive summary response had an unexpected type, report will omit it", "scan_id", data.ScanID)
		return
	}
	data.ExecutiveSummary = summary.Summary
	data.TopPriorities = summary.TopPriorities
}

func formatFindingCounts(counts map[domain.Severity]int) string {
	order := []domain.Severity{
		domain.SeverityCritical, domain.SeverityHigh, domain.SeverityMedium,
		domain.SeverityLow, domain.SeverityInformational,
	}
	var parts []string
	for _, sev := range order {
		if n, ok := counts[sev]; ok && n > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d", sev, n))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func formatTopFindings(findings []FindingRow, n int) string {
	if len(findings) == 0 {
		return "none"
	}
	if n > len(findings) {
		n = len(findings)
	}
	var lines []string
	for _, f := range findings[:n] {
		lines = append(lines, fmt.Sprintf("[%s] %s (%s, %s)", f.Severity, f.Title, f.Engine, f.Location))
	}
	return strings.Join(lines, "; ")
}

// formatEngineStatuses feeds ai.PromptSummariseScan's {{.engine_statuses}}
// var — deliberately including each job's coverage detail (open ports,
// phases run, ...) when present, not just its status/finding count. Without
// this, a clean scan gives the model nothing concrete to reference and it
// falls back to generic boilerplate ("indicates a strong security posture,"
// "maintain current practices") — the same complaint that motivated
// coverage.go in the first place applies here too: a clean run still did
// real, describable work, and the summary should be able to say so.
func formatEngineStatuses(jobs []JobSummary) string {
	if len(jobs) == 0 {
		return "none"
	}
	var parts []string
	for _, j := range jobs {
		part := fmt.Sprintf("%s: %s (%d findings)", j.Engine, j.Status, j.FindingCount)
		if j.Coverage != nil {
			part += " — " + formatCoverageForPrompt(j.Coverage)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

func formatCoverageForPrompt(cov *PentestCoverage) string {
	var parts []string
	if len(cov.OpenPorts) > 0 {
		ports := make([]string, len(cov.OpenPorts))
		for i, p := range cov.OpenPorts {
			ports[i] = fmt.Sprintf("%d", p)
		}
		parts = append(parts, fmt.Sprintf("open ports %s", strings.Join(ports, ",")))
	}
	if cov.HTTPServicesFound > 0 {
		parts = append(parts, fmt.Sprintf("%d HTTP service(s)", cov.HTTPServicesFound))
	}
	if cov.TLSPortsChecked > 0 {
		parts = append(parts, fmt.Sprintf("TLS checked on %d port(s)", cov.TLSPortsChecked))
	}
	if len(cov.TechnologiesFound) > 0 {
		parts = append(parts, fmt.Sprintf("technologies: %s", strings.Join(cov.TechnologiesFound, ",")))
	}
	if len(cov.NucleiCategoriesRun) > 0 {
		parts = append(parts, fmt.Sprintf("vuln signature categories: %s", strings.Join(cov.NucleiCategoriesRun, ",")))
	}
	if slices.Contains(cov.PhasesCompleted, "subdomain_enum") {
		parts = append(parts, fmt.Sprintf("%d additional subdomain(s) found", cov.SubdomainsFound))
	}
	parts = append(parts, fmt.Sprintf("%d checks across phases %s", cov.TotalScriptRuns, strings.Join(cov.PhasesCompleted, ",")))
	if len(cov.PhasesSkipped) > 0 {
		parts = append(parts, fmt.Sprintf("skipped %s", strings.Join(cov.PhasesSkipped, ",")))
	}
	return strings.Join(parts, "; ")
}
