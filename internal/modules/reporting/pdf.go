package reporting

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/page"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/border"
	"github.com/johnfercher/maroto/v2/pkg/consts/extension"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

//go:embed assets/logo.png
var logoPNG []byte

//go:embed assets/watermark.png
var watermarkPNG []byte

// severityColor gives each severity a distinct accent colour, used as a
// finding's severity label — the same worst-first palette a reader of any
// other GuardPipe surface (the dashboard's SeverityStatTile) already
// recognises.
var severityColor = map[domain.Severity]props.Color{
	domain.SeverityCritical:      {Red: 176, Green: 32, Blue: 32},
	domain.SeverityHigh:          {Red: 214, Green: 92, Blue: 20},
	domain.SeverityMedium:        {Red: 189, Green: 142, Blue: 10},
	domain.SeverityLow:           {Red: 51, Green: 102, Blue: 178},
	domain.SeverityInformational: {Red: 110, Green: 110, Blue: 110},
}

func colorFor(sev domain.Severity) *props.Color {
	if c, ok := severityColor[sev]; ok {
		return &c
	}
	c := severityColor[domain.SeverityInformational]
	return &c
}

var (
	darkText  = props.Color{Red: 30, Green: 32, Blue: 36}
	mutedText = props.Color{Red: 110, Green: 116, Blue: 122}
	brandBg   = props.Color{Red: 18, Green: 22, Blue: 28}
	white     = props.Color{Red: 255, Green: 255, Blue: 255}
	hairline  = props.Color{Red: 224, Green: 226, Blue: 230}
)

// RenderPDF builds a professional-report PDF from data, laid out as three
// deliberately-separated pages: a minimal cover (logo, project, target —
// nothing else), a scope/summary page (confidentiality notice, the full
// scan metadata table, the AI-authored executive summary, and per-engine
// coverage — the concrete answer to "don't just show vulnerabilities"), and
// the findings themselves grouped worst-first. Every page carries a faint
// logo + "GUARDPIPE CONFIDENTIAL" watermark (WithBackgroundImage below).
// Length is whatever the content actually needs — a clean small-target scan
// produces a short document, a scan with many findings across several
// engines produces a longer one; nothing here pads or truncates to hit a
// target page count.
func RenderPDF(data *ReportData) ([]byte, error) {
	cfg := config.NewBuilder().
		WithPageNumber(props.PageNumber{Pattern: "Page {current} of {total}", Place: props.RightBottom}).
		WithLeftMargin(12).
		WithRightMargin(12).
		WithTopMargin(10).
		WithBottomMargin(14).
		WithBackgroundImage(watermarkPNG, extension.Png).
		Build()

	mrt := maroto.New(cfg)

	mrt.AddPages(coverPage(data))
	mrt.AddPages(scopePage(data))
	if len(data.Findings) > 0 {
		mrt.AddPages(findingsPage(data))
	}

	doc, err := mrt.Generate()
	if err != nil {
		return nil, fmt.Errorf("reporting: generate pdf: %w", err)
	}
	return doc.GetBytes(), nil
}

func reportTitle(data *ReportData) string {
	if data.ScanType == domain.ScanTypePentestOnly {
		return "Penetration Test Report"
	}
	return "Security Scan Report"
}

// targetLabel is the cover page's one-line answer to "which website" — a
// pentest target's host when there is one, otherwise the repository this
// scan actually read.
func targetLabel(data *ReportData) string {
	if data.PentestTargetHost != "" {
		return data.PentestTargetHost
	}
	if data.RepositoryOwner != "" {
		return data.RepositoryOwner + "/" + data.RepositoryName
	}
	return "—"
}

// coverPage is deliberately minimal, per explicit direction: the logo, the
// report title, the project name, and the target/website — nothing else.
// Confidentiality marking lives in the watermark (every page, including
// this one) plus an explicit banner on the scope page below; the full
// metadata table, executive summary, and coverage all moved there too, so
// this page reads as a title page, not a second copy of page two.
func coverPage(data *ReportData) core.Page {
	p := page.New()

	p.Add(
		row.New(18).Add(
			col.New(2).Add(image.NewFromBytes(logoPNG, extension.Png)),
			text.NewCol(10, "GuardPipe", props.Text{Size: 12, Style: fontstyle.Bold, Color: &mutedText, Top: 6, Left: 2}),
		),
		row.New(16).Add(
			text.NewCol(12, reportTitle(data), props.Text{Size: 24, Style: fontstyle.Bold, Color: &darkText, Top: 4}),
		),
		row.New(10).Add(
			text.NewCol(12, data.ProjectName, props.Text{Size: 15, Color: &mutedText}),
		),
	)

	p.Add(
		row.New(14),
		metaRow("Scan", fmt.Sprintf("#%d", data.ScanNumber)),
		metaRow("Target", targetLabel(data)),
	)

	return p
}

// scopePage carries everything the cover page deliberately leaves out: the
// confidentiality notice, the full scan metadata table, the AI-authored
// executive summary, and per-engine coverage (merged into one page rather
// than split across two, per explicit direction).
func scopePage(data *ReportData) core.Page {
	p := page.New()

	p.Add(
		row.New(10).Add(
			col.New(12).
				WithStyle(&props.Cell{BackgroundColor: &brandBg}).
				Add(text.New(
					"CONFIDENTIAL — for the intended recipient only. Do not redistribute without authorisation.",
					props.Text{Size: 9, Style: fontstyle.Bold, Color: &white, Align: align.Center, Top: 3},
				)),
		),
		row.New(6),
	)

	p.Add(sectionHeading("Scope"))
	p.Add(scopeRows(data)...)

	if data.ExecutiveSummary != "" {
		p.Add(
			row.New(4),
			sectionHeading("Executive summary"),
			text.NewAutoRow(data.ExecutiveSummary, props.Text{Size: 10, Color: &darkText, Top: 2, Bottom: 3}),
		)
		if len(data.TopPriorities) > 0 {
			p.Add(text.NewAutoRow("Top priorities:", props.Text{Size: 10, Style: fontstyle.Bold, Color: &darkText, Top: 1}))
			for i, pr := range data.TopPriorities {
				p.Add(text.NewAutoRow(fmt.Sprintf("%d. %s", i+1, pr), props.Text{Size: 10, Color: &darkText, Left: 4, Top: 1}))
			}
		}
	}

	p.Add(row.New(6), sectionHeading("Coverage — what was checked"))
	p.Add(coverageRows(data)...)

	return p
}

// scopeRows is the scope page's metadata table — scan status/type,
// repository or target detail, scan window, engines run, and finding
// totals, so a reader knows exactly what this report does and does not
// cover before reading a single finding. Target/repository is repeated
// here (already on the cover) because this table needs to stand alone as
// the report's one complete scope statement.
func scopeRows(data *ReportData) []core.Row {
	rows := []core.Row{
		metaRow("Scan", fmt.Sprintf("#%d — %s", data.ScanNumber, string(data.ScanType))),
		metaRow("Status", string(data.ScanStatus)),
	}
	if data.RepositoryOwner != "" {
		ref := data.RepositoryOwner + "/" + data.RepositoryName
		if data.RepositoryBranch != "" {
			ref += " @ " + data.RepositoryBranch
		}
		rows = append(rows, metaRow("Repository", ref))
	}
	if data.CommitSHA != "" {
		rows = append(rows, metaRow("Commit", data.CommitSHA))
	}
	if data.PentestTargetHost != "" {
		rows = append(rows, metaRow("Target", data.PentestTargetHost))
	}
	rows = append(rows,
		metaRow("Scan window", scanWindow(data)),
		metaRow("Engines run", enginesList(data.Jobs)),
		metaRow("Findings", fmt.Sprintf("%d total — %s", data.TotalFindings, formatFindingCounts(data.FindingCounts))),
		metaRow("Report generated", data.GeneratedAt.Format("2006-01-02 15:04 MST")),
	)
	return rows
}

func scanWindow(data *ReportData) string {
	if data.StartedAt == nil {
		return "not started"
	}
	start := data.StartedAt.Format("2006-01-02 15:04:05 MST")
	if data.FinishedAt == nil {
		return start + " — in progress"
	}
	duration := data.FinishedAt.Sub(*data.StartedAt).Round(time.Second)
	return fmt.Sprintf("%s — %s (%s)", start, data.FinishedAt.Format("15:04:05 MST"), duration)
}

func enginesList(jobs []JobSummary) string {
	names := make([]string, len(jobs))
	for i, j := range jobs {
		names[i] = string(j.Engine)
	}
	return strings.Join(names, ", ")
}

func metaRow(label, value string) core.Row {
	return row.New(7).Add(
		text.NewCol(3, label, props.Text{Size: 9, Style: fontstyle.Bold, Color: &mutedText}),
		text.NewCol(9, value, props.Text{Size: 9, Color: &darkText}),
	)
}

func sectionHeading(title string) core.Row {
	return row.New(10).Add(text.NewCol(12, title, props.Text{Size: 14, Style: fontstyle.Bold, Color: &darkText, Top: 4}))
}

// coverageRows answers "what did this scan actually check" per engine —
// built from JobSummary/PentestCoverage, the same data
// internal/engines/pentest/coverage.go and the fixed worker.go persistence
// gap this feature started from now surface all the way to a client-facing
// report, not just the live UI. Returns rows (not a page) so scopePage can
// embed it directly under the executive summary, per explicit direction
// that scope/summary/coverage all belong together on one page.
func coverageRows(data *ReportData) []core.Row {
	var rows []core.Row
	for _, j := range data.Jobs {
		rows = append(rows, row.New(2))
		rows = append(rows, row.New(7).Add(
			text.NewCol(4, string(j.Engine), props.Text{Size: 11, Style: fontstyle.Bold, Color: &darkText}),
			text.NewCol(4, jobStatusLabel(j), props.Text{Size: 9, Color: &mutedText}),
			text.NewCol(4, fmt.Sprintf("%d finding(s)", j.FindingCount), props.Text{Size: 9, Color: &mutedText, Align: align.Right}),
		))
		if j.SkipReason != "" {
			rows = append(rows, text.NewAutoRow("Skipped: "+j.SkipReason, props.Text{Size: 9, Color: &mutedText, Left: 4}))
		}
		if j.ErrorReason != "" {
			rows = append(rows, text.NewAutoRow("Failed: "+j.ErrorReason, props.Text{Size: 9, Color: &mutedText, Left: 4}))
		}
		if j.Coverage != nil {
			rows = append(rows, coverageDetailRows(j.Coverage)...)
		}
		rows = append(rows, row.New(3).Add(col.New(12).WithStyle(&props.Cell{BorderColor: &hairline, BorderType: border.Bottom})))
	}
	return rows
}

func jobStatusLabel(j JobSummary) string {
	label := string(j.Status)
	if j.StartedAt != nil && j.FinishedAt != nil {
		label += fmt.Sprintf(" · took %s", j.FinishedAt.Sub(*j.StartedAt).Round(time.Second))
	}
	return label
}

// coverageDetailRows renders pentest's coverage summary as a compact,
// concrete list — this is the report-generation half of the same fix that
// stopped a clean pentest run from showing "0 files scanned, 0 rules
// evaluated, 0 findings" in the live UI: the report gets the identical real
// data, not a re-derived or re-worded version of it.
func coverageDetailRows(cov *PentestCoverage) []core.Row {
	var rows []core.Row
	add := func(label, value string) {
		if value == "" {
			return
		}
		rows = append(rows, text.NewAutoRow(label+": "+value, props.Text{Size: 9, Color: &darkText, Left: 4, Top: 1}))
	}
	add("Open ports", joinInts(cov.OpenPorts))
	add("HTTP services probed", fmt.Sprintf("%d", cov.HTTPServicesFound))
	if cov.TLSPortsChecked > 0 {
		add("TLS ports checked", fmt.Sprintf("%d", cov.TLSPortsChecked))
	}
	if len(cov.TechnologiesFound) > 0 {
		add("Technologies detected", strings.Join(cov.TechnologiesFound, ", "))
	}
	add("Vulnerability signature categories checked", strings.Join(cov.NucleiCategoriesRun, ", "))
	if cov.CrawledPathsFound > 0 {
		add("Paths discovered by crawl", fmt.Sprintf("%d", cov.CrawledPathsFound))
	}
	add("Total checks run", fmt.Sprintf("%d", cov.TotalScriptRuns))
	add("Phases completed", strings.Join(cov.PhasesCompleted, ", "))
	if len(cov.PhasesSkipped) > 0 {
		add("Phases skipped", strings.Join(cov.PhasesSkipped, ", "))
	}
	return rows
}

func joinInts(ints []int) string {
	if len(ints) == 0 {
		return "none found"
	}
	parts := make([]string, len(ints))
	for i, n := range ints {
		parts[i] = fmt.Sprintf("%d", n)
	}
	return strings.Join(parts, ", ")
}

// findingsPage lists every finding worst-severity-first (Assembler.Build
// already sorts them). Each finding gets its own colour-coded severity
// label plus a deterministic, rule-authored remediation — never an AI
// paraphrase, per the same "must stand alone without AI" rule every other
// GuardPipe surface follows.
func findingsPage(data *ReportData) core.Page {
	p := page.New()
	p.Add(sectionHeading(fmt.Sprintf("Findings (%d)", len(data.Findings))))

	for _, f := range data.Findings {
		p.Add(row.New(2))
		p.Add(row.New(7).Add(
			col.New(2).
				WithStyle(&props.Cell{BackgroundColor: colorFor(f.Severity)}).
				Add(text.New(strings.ToUpper(string(f.Severity)), props.Text{Size: 8, Style: fontstyle.Bold, Color: &white, Align: align.Center, Top: 2.5})),
			text.NewCol(10, f.Title, props.Text{Size: 11, Style: fontstyle.Bold, Color: &darkText, Left: 3, Top: 1}),
		))
		p.Add(text.NewAutoRow(findingMetaLine(f), props.Text{Size: 8, Color: &mutedText, Top: 1}))
		if f.Description != "" {
			p.Add(text.NewAutoRow(f.Description, props.Text{Size: 9, Color: &darkText, Top: 1}))
		}
		if f.Remediation != "" {
			p.Add(text.NewAutoRow("Remediation: "+f.Remediation, props.Text{Size: 9, Color: &darkText, Top: 1, Style: fontstyle.Italic}))
		}
		p.Add(row.New(3).Add(col.New(12).WithStyle(&props.Cell{BorderColor: &hairline, BorderType: border.Bottom})))
	}

	return p
}

func findingMetaLine(f FindingRow) string {
	parts := []string{string(f.Engine), f.RuleID, f.Location}
	if len(f.CWE) > 0 {
		parts = append(parts, strings.Join(f.CWE, ", "))
	}
	if len(f.CVE) > 0 {
		parts = append(parts, strings.Join(f.CVE, ", "))
	}
	return strings.Join(parts, "  ·  ")
}
