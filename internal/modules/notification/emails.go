package notification

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// Every value interpolated into these templates that came from a user or a
// repository (project names, branch names) goes through html/template's
// contextual escaping in the HTML part — a project called
// "<img src=x onerror=...>" arrives as text, not markup.

var severityOrder = []domain.Severity{
	domain.SeverityCritical, domain.SeverityHigh, domain.SeverityMedium,
	domain.SeverityLow, domain.SeverityInformational,
}

type severityCount struct {
	Label string
	Count int
	Color string
}

var severityColor = map[domain.Severity]string{
	domain.SeverityCritical:      "#b91c1c",
	domain.SeverityHigh:          "#c2410c",
	domain.SeverityMedium:        "#a16207",
	domain.SeverityLow:           "#1d4ed8",
	domain.SeverityInformational: "#4b5563",
}

type reportEmailData struct {
	ProjectName  string
	ScanNumber   int
	StatusLabel  string
	Failed       bool
	Score        *int
	Verdict      string
	Severities   []severityCount
	Total        int
	Origin       string
	Link         string
	Attached     bool
	TooLargeNote bool
}

var reportHTML = htmltemplate.Must(htmltemplate.New("report").Parse(`<!doctype html>
<html><body style="margin:0;padding:24px;background:#f4f5f7;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#172b4d">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;margin:0 auto;background:#ffffff;border-radius:8px;border:1px solid #dfe1e6">
<tr><td style="padding:20px 24px;border-bottom:1px solid #dfe1e6;font-weight:600;font-size:16px">GuardPipe</td></tr>
<tr><td style="padding:24px">
<p style="margin:0 0 4px;font-size:13px;color:#5e6c84">{{.ProjectName}} · Scan #{{.ScanNumber}}</p>
<h1 style="margin:0 0 16px;font-size:20px">{{.StatusLabel}}</h1>
{{if .Score}}<p style="margin:0 0 16px;font-size:15px">Risk score <strong style="font-size:22px">{{.Score}}</strong> / 100 · verdict <strong>{{.Verdict}}</strong></p>{{end}}
{{if .Failed}}<p style="margin:0 0 16px;font-size:14px">The scan didn't complete. Open it in GuardPipe to see which engine failed and why — any engines that did finish still reported their findings.</p>{{end}}
<table role="presentation" cellpadding="0" cellspacing="0" style="margin:0 0 16px;font-size:14px">
{{range .Severities}}<tr><td style="padding:2px 16px 2px 0;color:{{.Color}};font-weight:600">{{.Label}}</td><td style="padding:2px 0">{{.Count}}</td></tr>{{end}}
<tr><td style="padding:6px 16px 2px 0;font-weight:600">Total findings</td><td style="padding:6px 0 2px">{{.Total}}</td></tr>
</table>
{{if .Origin}}<p style="margin:0 0 16px;font-size:13px;color:#5e6c84">{{.Origin}}</p>{{end}}
<p style="margin:0 0 20px"><a href="{{.Link}}" style="display:inline-block;background:#0c66e4;color:#ffffff;text-decoration:none;padding:10px 16px;border-radius:6px;font-weight:600;font-size:14px">View results in GuardPipe</a></p>
{{if .Attached}}<p style="margin:0;font-size:13px;color:#5e6c84">The full PDF report is attached.</p>{{end}}
{{if .TooLargeNote}}<p style="margin:0;font-size:13px;color:#5e6c84">The PDF report was too large to attach — download it from the scan page.</p>{{end}}
</td></tr>
<tr><td style="padding:16px 24px;border-top:1px solid #dfe1e6;font-size:12px;color:#5e6c84">This report contains security findings about your code. Treat it as confidential. You're getting this because you started this scan (or set up the schedule or live scanning that did); change where reports go, or turn them off, in GuardPipe → Settings → Notifications.</td></tr>
</table></body></html>`))

var reportText = texttemplate.Must(texttemplate.New("report").Parse(`{{.StatusLabel}} — {{.ProjectName}} · Scan #{{.ScanNumber}}
{{if .Score}}
Risk score: {{.Score}} / 100 (verdict: {{.Verdict}})
{{end}}{{if .Failed}}
The scan didn't complete. Open it in GuardPipe to see which engine failed and why.
{{end}}
{{range .Severities}}{{.Label}}: {{.Count}}
{{end}}Total findings: {{.Total}}
{{if .Origin}}
{{.Origin}}
{{end}}
View results: {{.Link}}
{{if .Attached}}
The full PDF report is attached.
{{end}}{{if .TooLargeNote}}
The PDF report was too large to attach — download it from the scan page.
{{end}}
--
This report contains security findings about your code. Treat it as confidential.
Change where reports go, or turn them off, in GuardPipe -> Settings -> Notifications.
`))

func reportEmail(to string, info *ScanInfo, link string, pdf []byte, attach bool) (Email, error) {
	data := reportEmailData{
		ProjectName:  info.ProjectName,
		ScanNumber:   info.ScanNumber,
		Failed:       info.Status == domain.ScanStatusFailed,
		Link:         link,
		Attached:     attach && len(pdf) > 0,
		TooLargeNote: !attach && len(pdf) > 0,
	}
	if data.Failed {
		data.StatusLabel = "Scan failed"
	} else {
		data.StatusLabel = "Scan finished"
	}
	if info.Score != nil && info.Verdict != nil {
		data.Score = info.Score
		data.Verdict = *info.Verdict
	}
	for _, sev := range severityOrder {
		n := info.FindingCounts[sev]
		data.Total += n
		data.Severities = append(data.Severities, severityCount{Label: capitalize(string(sev)), Count: n, Color: severityColor[sev]})
	}
	data.Origin = originLine(info)

	var html, text bytes.Buffer
	if err := reportHTML.Execute(&html, data); err != nil {
		return Email{}, fmt.Errorf("render report email html: %w", err)
	}
	if err := reportText.Execute(&text, data); err != nil {
		return Email{}, fmt.Errorf("render report email text: %w", err)
	}

	subject := fmt.Sprintf("[GuardPipe] %s · Scan #%d — ", singleLine(info.ProjectName), info.ScanNumber)
	switch {
	case data.Failed:
		subject += "scan failed"
	case data.Score != nil:
		subject += fmt.Sprintf("risk score %d (%s)", *data.Score, data.Verdict)
	default:
		subject += "scan finished"
	}

	e := Email{To: to, Subject: subject, HTML: html.String(), Text: text.String()}
	if data.Attached {
		e.Attachments = []Attachment{{
			Filename:    fmt.Sprintf("guardpipe-scan-%d.pdf", info.ScanNumber),
			ContentType: "application/pdf",
			Data:        pdf,
		}}
	}
	return e, nil
}

func originLine(info *ScanInfo) string {
	switch info.TriggerSource {
	case domain.TriggerWebhookPush:
		if info.TriggerRef != nil {
			return "Live scan — started by a push to " + *info.TriggerRef
		}
		return "Live scan — started by a push"
	case domain.TriggerWebhookPullRequest:
		return "Live scan — started by a pull request"
	case domain.TriggerScheduled:
		return "Scheduled scan"
	}
	if info.Branch != nil && *info.Branch != "" {
		return "Branch " + *info.Branch
	}
	return ""
}

type linkEmailData struct {
	Name string
	Link string
	To   string
}

var verifyHTML = htmltemplate.Must(htmltemplate.New("verify").Parse(`<!doctype html>
<html><body style="margin:0;padding:24px;background:#f4f5f7;font-family:-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif;color:#172b4d">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;margin:0 auto;background:#ffffff;border-radius:8px;border:1px solid #dfe1e6">
<tr><td style="padding:24px">
<h1 style="margin:0 0 16px;font-size:20px">Confirm your report address</h1>
<p style="margin:0 0 16px;font-size:14px">{{if .Name}}{{.Name}} asked{{else}}Someone asked{{end}} GuardPipe to send scan reports to <strong>{{.To}}</strong>. Reports will only go here once you confirm.</p>
<p style="margin:0 0 20px"><a href="{{.Link}}" style="display:inline-block;background:#0c66e4;color:#ffffff;text-decoration:none;padding:10px 16px;border-radius:6px;font-weight:600;font-size:14px">Confirm this address</a></p>
<p style="margin:0;font-size:12px;color:#5e6c84">The link works for 24 hours. If you didn't expect this, ignore it — nothing will be sent to you.</p>
</td></tr></table></body></html>`))

var verifyText = texttemplate.Must(texttemplate.New("verify").Parse(`Confirm your report address

{{if .Name}}{{.Name}} asked{{else}}Someone asked{{end}} GuardPipe to send scan reports to {{.To}}. Reports will only go here once you confirm:

{{.Link}}

The link works for 24 hours. If you didn't expect this, ignore it — nothing will be sent to you.
`))

func verificationEmail(to, displayName, link string) Email {
	data := linkEmailData{Name: singleLine(displayName), Link: link, To: to}
	var html, text bytes.Buffer
	// Execution can only fail on a template bug; the fixed templates above
	// are exercised by the tests, so a failure here would be caught there.
	_ = verifyHTML.Execute(&html, data)
	_ = verifyText.Execute(&text, data)
	return Email{To: to, Subject: "[GuardPipe] Confirm your report address", HTML: html.String(), Text: text.String()}
}

func testEmail(to, appURL string) Email {
	text := "This is a test email from GuardPipe. Scan reports will be delivered to this address.\n\n" + appURL + "\n"
	var html bytes.Buffer
	_ = htmltemplate.Must(htmltemplate.New("test").Parse(
		`<p style="font-family:sans-serif;font-size:14px">This is a test email from GuardPipe. Scan reports will be delivered to this address.</p><p><a href="{{.}}">Open GuardPipe</a></p>`,
	)).Execute(&html, appURL)
	return Email{To: to, Subject: "[GuardPipe] Test email", Text: text, HTML: html.String()}
}

// singleLine keeps a user-controlled value from injecting extra lines into
// a subject (the mailer also rejects CR/LF in headers; this keeps the
// subject readable instead of failing the send).
func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
