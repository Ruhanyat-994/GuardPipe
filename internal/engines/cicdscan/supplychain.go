package cicdscan

import (
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// fullSHA matches a full 40-character commit SHA — the only ref shape
// cicdscan.supply-chain.unpinned-action treats as pinned. A short SHA
// (7-8 chars) is deliberately still flagged: it's not ambiguous today, but
// it isn't the full, unambiguous identifier GitHub's own hardening guidance
// asks for either.
var fullSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

// trustedActionPublishers is a curated, deliberately incomplete allowlist
// for cicdscan.supply-chain.unverified-action — GitHub's own namespace, the
// major cloud providers' official Actions orgs, and a handful of
// widely-used community publishers. Not being on this list is a "couldn't
// confirm," medium-confidence signal, not a claim that the action is
// actually malicious — an unfamiliar-but-legitimate publisher is exactly
// the false-positive case Confidence: medium already accounts for.
var trustedActionPublishers = map[string]bool{
	"actions": true, "github": true, "docker": true,
	"azure": true, "Azure": true, "aws-actions": true, "google-github-actions": true,
	"hashicorp": true, "actions-rs": true, "codecov": true, "softprops": true,
	"peter-evans": true, "dorny": true, "EndBug": true, "sigstore": true,
}

var curlPipeShell = regexp.MustCompile(`(?i)\b(curl|wget)\b[^\n|]*\|\s*(sudo\s+)?(bash|sh|zsh)\b`)

var (
	goInstallLatest = regexp.MustCompile(`\bgo install\s+\S+@latest\b`)
	npmInstallNoVer = regexp.MustCompile(`\bnpm install\s+(-g\s+)?[a-zA-Z@/][\w@/.-]*\b`)
	pipInstallLine  = regexp.MustCompile(`\bpip3?\s+install\b[^\n]*`)
)

func evaluateSupplyChain(scanID uuid.UUID, wf Workflow) []domain.Finding {
	var findings []domain.Finding
	for _, j := range wf.Jobs {
		for _, s := range j.Steps {
			if s.Uses != "" {
				findings = append(findings, evaluateActionReference(scanID, wf, j, s)...)
			}
			if s.Run != "" {
				findings = append(findings, evaluateRunStep(scanID, wf, j, s)...)
			}
		}
	}
	return findings
}

// evaluateActionReference covers unpinned-action and unverified-action —
// both look at the same `uses:` reference, so they're evaluated together
// rather than re-parsing it twice.
func evaluateActionReference(scanID uuid.UUID, wf Workflow, j Job, s Step) []domain.Finding {
	// Local actions (./path) and Docker-image actions (docker://...) have no
	// "org/repo@ref" shape at all — out of scope for both rules.
	if strings.HasPrefix(s.Uses, "./") || strings.HasPrefix(s.Uses, "docker://") {
		return nil
	}

	var findings []domain.Finding
	ref := s.Uses
	org, rest, hasSlash := strings.Cut(s.Uses, "/")
	if !hasSlash {
		return nil // not a recognisable org/repo[@ref] shape
	}

	if at := strings.LastIndex(rest, "@"); at >= 0 {
		version := rest[at+1:]
		if !fullSHA.MatchString(version) {
			findings = append(findings, stepFinding(scanID, wf, j, s, "cicdscan.supply-chain.unpinned-action", ref))
		}
	} else {
		// No @ref at all — implicitly the action's default branch, strictly
		// worse than a named mutable tag.
		findings = append(findings, stepFinding(scanID, wf, j, s, "cicdscan.supply-chain.unpinned-action", ref))
	}

	if !trustedActionPublishers[org] {
		findings = append(findings, stepFinding(scanID, wf, j, s, "cicdscan.supply-chain.unverified-action", ref))
	}
	return findings
}

func evaluateRunStep(scanID uuid.UUID, wf Workflow, j Job, s Step) []domain.Finding {
	var findings []domain.Finding
	if curlPipeShell.MatchString(s.Run) {
		findings = append(findings, stepFinding(scanID, wf, j, s, "cicdscan.supply-chain.curl-pipe-shell", curlPipeShell.FindString(s.Run)))
	}
	if evidence, ok := unpinnedInstall(s.Run); ok {
		findings = append(findings, stepFinding(scanID, wf, j, s, "cicdscan.supply-chain.unpinned-install", evidence))
	}
	return findings
}

// unpinnedInstall is a deliberately narrow heuristic over three common
// ecosystems' install commands, not an exhaustive package-manager parser:
// `go install x@latest`, `npm install`/`npm install -g` with no `@version`
// suffix, and `pip install` with neither a `==` pin nor a `-r
// requirements.txt` reference. False negatives (an install pattern this
// doesn't recognise) are expected and acceptable — this rule's job is to
// catch the common, careless case, not every possible one.
func unpinnedInstall(run string) (evidence string, ok bool) {
	if m := goInstallLatest.FindString(run); m != "" {
		return m, true
	}
	if m := npmInstallNoVer.FindString(run); m != "" && !strings.Contains(m, "@") && !strings.Contains(run, "npm ci") {
		return m, true
	}
	if m := pipInstallLine.FindString(run); m != "" && !strings.Contains(m, "==") && !strings.Contains(m, "-r ") {
		return m, true
	}
	return "", false
}
