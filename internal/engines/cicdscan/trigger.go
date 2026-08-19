package cicdscan

import (
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// prHeadCheckout matches the expressions that name the pull request's own
// head commit — the canonical shape is a checkout step's `with: ref:`
// pointing at it, but the same expression sometimes appears in a later
// run: step instead (a manual `git fetch`/`git checkout`), so both are
// checked.
var prHeadCheckout = regexp.MustCompile(`github\.event\.pull_request\.head\.(sha|ref)`)

// scriptInjectionEventPath matches `${{ github.event.<anything> }}`
// interpolated inline — documentation/05-module-specifications.md §10's own
// wording ("`${{ github.event.* }}` interpolated directly into `run:`"),
// deliberately not narrowed to only the well-known attacker-controlled
// fields (issue.title, pull_request.title/body, ...): any github.event.*
// field can carry attacker-influenced text depending on the trigger, and a
// narrower allowlist would silently miss less obvious ones.
var scriptInjectionEventPath = regexp.MustCompile(`\$\{\{\s*github\.event\.[a-zA-Z0-9_.]+\s*\}\}`)

func evaluateTriggers(scanID uuid.UUID, wf Workflow) []domain.Finding {
	var findings []domain.Finding

	if slices.Contains(wf.Triggers, "pull_request_target") {
		findings = append(findings, evaluatePullRequestTargetCheckout(scanID, wf)...)
	}
	if wf.WorkflowRunDownloadsArtifact {
		findings = append(findings, workflowFinding(scanID, wf, "cicdscan.trigger.workflow-run-untrusted", "workflow_run+download-artifact"))
	}
	for _, j := range wf.Jobs {
		for _, s := range j.Steps {
			if s.Run == "" {
				continue
			}
			if m := scriptInjectionEventPath.FindString(s.Run); m != "" {
				findings = append(findings, stepFinding(scanID, wf, j, s, "cicdscan.injection.script-injection", m))
			}
		}
	}
	return findings
}

// evaluatePullRequestTargetCheckout looks for the PR's own head commit
// named either in a checkout step's `with: ref:` (the canonical, most
// common real-world shape) or anywhere in a later run: step (a manual
// git checkout of the same ref).
func evaluatePullRequestTargetCheckout(scanID uuid.UUID, wf Workflow) []domain.Finding {
	var findings []domain.Finding
	for _, j := range wf.Jobs {
		for _, s := range j.Steps {
			if s.Uses != "" && strings.HasPrefix(s.Uses, "actions/checkout") {
				if ref, ok := s.With["ref"]; ok && prHeadCheckout.MatchString(ref) {
					findings = append(findings, stepFinding(scanID, wf, j, s, "cicdscan.trigger.pull-request-target-checkout", ref))
				}
				continue
			}
			if s.Run != "" {
				if m := prHeadCheckout.FindString(s.Run); m != "" {
					findings = append(findings, stepFinding(scanID, wf, j, s, "cicdscan.trigger.pull-request-target-checkout", m))
				}
			}
		}
	}
	return findings
}
