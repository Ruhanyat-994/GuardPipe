package cicdscan

import (
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// githubHostedLabel matches GitHub's own hosted-runner label families —
// anything else on runs-on: (bare "self-hosted", or a custom label like
// "gpu-runner") is either explicitly or implicitly a self-hosted runner.
var githubHostedLabel = regexp.MustCompile(`^(ubuntu|windows|macos|macOS)-`)

// forkReachableTriggers are `on:` events an external contributor (someone
// with no write access, via a fork) can cause to run.
var forkReachableTriggers = []string{"pull_request", "issue_comment", "pull_request_review", "pull_request_review_comment"}

var sensitiveArtifactPath = regexp.MustCompile(`(?i)(\.env\b|\.pem\b|\.key\b|id_rsa|credentials)`)

func evaluateRunner(scanID uuid.UUID, wf Workflow) []domain.Finding {
	var findings []domain.Finding

	forkReachable := false
	for _, t := range wf.Triggers {
		if slices.Contains(forkReachableTriggers, t) {
			forkReachable = true
			break
		}
	}

	for _, j := range wf.Jobs {
		if forkReachable && usesSelfHostedRunner(j.RunsOn) {
			findings = append(findings, jobFinding(scanID, wf, j, "cicdscan.runner.self-hosted-public", strings.Join(j.RunsOn, ",")))
		}
		if j.Container != "" && !strings.Contains(j.Container, "@sha256:") {
			findings = append(findings, jobFinding(scanID, wf, j, "cicdscan.runner.unpinned-image", j.Container))
		}
		for _, s := range j.Steps {
			if s.Uses == "" || !strings.HasPrefix(s.Uses, "actions/upload-artifact") {
				continue
			}
			if path, ok := s.With["path"]; ok {
				if m := sensitiveArtifactPath.FindString(path); m != "" {
					findings = append(findings, stepFinding(scanID, wf, j, s, "cicdscan.artifact.upload-sensitive", m))
				}
			}
		}
	}
	return findings
}

// usesSelfHostedRunner reports whether any of runsOn is a self-hosted or
// custom label. A `${{ ... }}` expression (the common `runs-on: ${{
// matrix.os }}` shape) is deliberately never treated as self-hosted here,
// even though it doesn't match githubHostedLabel either: its actual
// resolved value is a matrix/expression result this engine can't evaluate
// statically, and — confirmed against a real repository during this rule's
// own verification — the matrix values behind such an expression are
// routinely all standard GitHub-hosted labels (e.g. `[ubuntu-latest,
// macos-latest, windows-latest]`). Treating "can't tell" as "assume
// self-hosted" produced exactly that false positive; treating it as "can't
// tell, so don't flag it" is the safer default for a rule already at
// Confidence: medium.
func usesSelfHostedRunner(runsOn []string) bool {
	for _, label := range runsOn {
		if strings.Contains(label, "${{") {
			continue
		}
		if !githubHostedLabel.MatchString(label) {
			return true
		}
	}
	return false
}
