package cicdscan

import (
	"regexp"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

var (
	secretEchoed    = regexp.MustCompile(`(?i)\b(echo|print)\b[^\n]*\$\{\{\s*secrets\.[A-Za-z0-9_]+\s*\}\}`)
	secretReference = regexp.MustCompile(`secrets\.[A-Za-z0-9_]+`)
)

func evaluateSecrets(scanID uuid.UUID, wf Workflow) []domain.Finding {
	var findings []domain.Finding
	for _, j := range wf.Jobs {
		// secrets: inherit is only valid syntax on a job that calls a
		// reusable workflow (uses: at the job level) — checked anyway
		// rather than trusted, since a hand-written fixture or a real
		// workflow with a typo could set both.
		if j.SecretsInherit && j.Uses != "" {
			findings = append(findings, jobFinding(scanID, wf, j, "cicdscan.secrets.inherit", "secrets: inherit"))
		}
		for _, s := range j.Steps {
			if s.Run != "" {
				if m := secretEchoed.FindString(s.Run); m != "" {
					findings = append(findings, stepFinding(scanID, wf, j, s, "cicdscan.secrets.echoed", m))
				}
			}
			if s.If != "" {
				if m := secretReference.FindString(s.If); m != "" {
					findings = append(findings, stepFinding(scanID, wf, j, s, "cicdscan.secrets.in-condition", m))
				}
			}
		}
	}
	return findings
}
