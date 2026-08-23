package reporting

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
)

var findingsCSVHeader = []string{
	"engine", "rule_id", "severity", "confidence", "title",
	"location", "cwe", "cve", "remediation",
}

// RenderCSV renders a report as three sections — scan metadata, per-engine
// coverage, then the findings table — separated by a blank line, the same
// convention spreadsheet exports commonly use (e.g. Google Analytics' own
// CSV export) so Excel/Sheets still opens it as one file while each section
// stays a clean, independently-parseable table. Earlier versions emitted
// only the findings table, which meant a clean scan's CSV was just a bare
// header row — this is what fixes that: a 0-finding scan's CSV still shows
// exactly what was checked, the same information the PDF's coverage page
// and the live UI already show.
func RenderCSV(data *ReportData) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	if err := writeScanInfoSection(w, data); err != nil {
		return nil, err
	}
	if err := w.Write(nil); err != nil {
		return nil, err
	}
	if err := writeCoverageSection(w, data); err != nil {
		return nil, err
	}
	if err := w.Write(nil); err != nil {
		return nil, err
	}
	if err := writeFindingsSection(w, data); err != nil {
		return nil, err
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeScanInfoSection(w *csv.Writer, data *ReportData) error {
	rows := [][]string{
		{"Scan Report"},
		{"Project", data.ProjectName},
	}
	if data.RepositoryOwner != "" {
		rows = append(rows, []string{"Repository", data.RepositoryOwner + "/" + data.RepositoryName})
	}
	if data.PentestTargetHost != "" {
		rows = append(rows, []string{"Target", data.PentestTargetHost})
	}
	rows = append(rows,
		[]string{"Scan Number", fmt.Sprintf("#%d", data.ScanNumber)},
		[]string{"Scan Type", string(data.ScanType)},
		[]string{"Status", string(data.ScanStatus)},
		[]string{"Scan Window", scanWindow(data)},
		[]string{"Findings", fmt.Sprintf("%d total (%s)", data.TotalFindings, formatFindingCounts(data.FindingCounts))},
		[]string{"Report Generated", data.GeneratedAt.Format("2006-01-02 15:04 MST")},
	)
	for _, r := range rows {
		if err := w.Write(r); err != nil {
			return err
		}
	}
	return nil
}

func writeCoverageSection(w *csv.Writer, data *ReportData) error {
	if err := w.Write([]string{"Coverage"}); err != nil {
		return err
	}
	if err := w.Write([]string{"engine", "status", "duration", "finding_count", "detail"}); err != nil {
		return err
	}
	for _, j := range data.Jobs {
		detail := ""
		switch {
		case j.Coverage != nil:
			detail = formatCoverageForPrompt(j.Coverage)
		case j.SkipReason != "":
			detail = "skipped: " + j.SkipReason
		case j.ErrorReason != "":
			detail = "failed: " + j.ErrorReason
		}
		duration := ""
		if j.StartedAt != nil && j.FinishedAt != nil {
			duration = j.FinishedAt.Sub(*j.StartedAt).String()
		}
		row := []string{string(j.Engine), string(j.Status), duration, fmt.Sprintf("%d", j.FindingCount), detail}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func writeFindingsSection(w *csv.Writer, data *ReportData) error {
	if err := w.Write([]string{"Findings"}); err != nil {
		return err
	}
	if err := w.Write(findingsCSVHeader); err != nil {
		return err
	}
	for _, f := range data.Findings {
		row := []string{
			string(f.Engine), f.RuleID, string(f.Severity), string(f.Confidence), f.Title,
			f.Location, strings.Join(f.CWE, "; "), strings.Join(f.CVE, "; "), f.Remediation,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}
