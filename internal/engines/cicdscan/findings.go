package cicdscan

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// rulesByID is built once at package init from Rules, mirroring every other
// engine's own "declare once, reuse at emit time" shape (k8sscan.rulesByID,
// depscan's own rule lookup).
var rulesByID = func() map[string]domain.RuleMeta {
	m := make(map[string]domain.RuleMeta, len(Rules))
	for _, r := range Rules {
		m[r.ID] = r
	}
	return m
}()

func ruleMetaByID(ruleID string) domain.RuleMeta {
	meta, ok := rulesByID[ruleID]
	if !ok {
		panic("cicdscan: unknown rule ID " + ruleID + " — add it to Rules in rules.go")
	}
	return meta
}

// workflowFinding builds one Finding for a workflow-level rule hit (a
// property of the whole file — missing permissions:, a risky trigger
// combination — not any one job or step).
func workflowFinding(scanID uuid.UUID, wf Workflow, ruleID, evidence string) domain.Finding {
	meta := ruleMetaByID(ruleID)
	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineCICDScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, wf.File, evidence),
		Title:       fmt.Sprintf("%s (%s)", meta.Title, wf.File),
		Description: meta.Description,
		Severity:    meta.Severity, Confidence: meta.Confidence,
		CWE:         meta.CWE,
		Location:    domain.Location{Type: domain.LocationTypeFile, Path: wf.File, LineStart: wf.Line},
		Remediation: meta.Remediation,
		Status:      domain.StatusOpen,
		Source:      domain.FindingSourceRule,
	}
}

// jobFinding builds one Finding for a job-level rule hit.
func jobFinding(scanID uuid.UUID, wf Workflow, j Job, ruleID, evidence string) domain.Finding {
	meta := ruleMetaByID(ruleID)
	normLoc := wf.File + "|job:" + j.ID
	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineCICDScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, normLoc, evidence),
		Title:       fmt.Sprintf("%s (%s, job %q)", meta.Title, wf.File, j.ID),
		Description: meta.Description,
		Severity:    meta.Severity, Confidence: meta.Confidence,
		CWE:         meta.CWE,
		Location:    domain.Location{Type: domain.LocationTypeFile, Path: wf.File, LineStart: j.Line},
		Remediation: meta.Remediation,
		Status:      domain.StatusOpen,
		Source:      domain.FindingSourceRule,
	}
}

// stepFinding builds one Finding for a step-level rule hit — most of the 16
// Core rules are this shape.
func stepFinding(scanID uuid.UUID, wf Workflow, j Job, s Step, ruleID, evidence string) domain.Finding {
	meta := ruleMetaByID(ruleID)
	stepName := s.Name
	if stepName == "" {
		stepName = firstNonEmpty(s.Uses, "run step")
	}
	normLoc := fmt.Sprintf("%s|job:%s|step:%s", wf.File, j.ID, stepName)
	return domain.Finding{
		ID: id.New(), ScanID: scanID, Engine: domain.EngineCICDScan, RuleID: ruleID,
		Fingerprint: id.Fingerprint(ruleID, normLoc, evidence),
		Title:       fmt.Sprintf("%s (%s, job %q, step %q)", meta.Title, wf.File, j.ID, stepName),
		Description: meta.Description,
		Severity:    meta.Severity, Confidence: meta.Confidence,
		CWE:         meta.CWE,
		Location:    domain.Location{Type: domain.LocationTypeFile, Path: wf.File, LineStart: s.Line},
		Remediation: meta.Remediation,
		Status:      domain.StatusOpen,
		Source:      domain.FindingSourceRule,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
