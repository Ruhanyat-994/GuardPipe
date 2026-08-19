package cicdscan

import (
	"strings"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// writeLikeKeywords is cicdscan.permissions.excessive-token's heuristic for
// "this job's steps look like they write to the repository" — checked
// against every step's uses:/run: text, lowercased. Deliberately broad
// (matching "release" catches actions/create-release,
// softprops/action-gh-release, and a plain `run: gh release create` alike)
// rather than an exhaustive action-name allowlist that would need updating
// every time a new publish/deploy action becomes popular.
var writeLikeKeywords = []string{"push", "commit", "release", "publish", "deploy", "upload"}

func evaluatePermissions(scanID uuid.UUID, wf Workflow) []domain.Finding {
	var findings []domain.Finding

	if !wf.Permissions.Present {
		findings = append(findings, workflowFinding(scanID, wf, "cicdscan.permissions.missing-block", "no top-level permissions:"))
	}
	if wf.Permissions.WriteAll {
		findings = append(findings, workflowFinding(scanID, wf, "cicdscan.permissions.write-all", "write-all"))
	}

	for _, j := range wf.Jobs {
		if j.Permissions.WriteAll {
			findings = append(findings, jobFinding(scanID, wf, j, "cicdscan.permissions.write-all", "write-all"))
		}
		if hasExcessiveContentsWrite(wf, j) {
			findings = append(findings, jobFinding(scanID, wf, j, "cicdscan.permissions.excessive-token", "contents:write"))
		}
	}
	return findings
}

// hasExcessiveContentsWrite reports whether j (using its own permissions:
// block if it has one, otherwise the workflow's) grants contents: write
// while none of its steps look like they write to the repository.
func hasExcessiveContentsWrite(wf Workflow, j Job) bool {
	effective := j.Permissions
	if !effective.Present {
		effective = wf.Permissions
	}
	if !effective.Present || effective.ReadAll {
		return false
	}
	grantsContentsWrite := effective.WriteAll
	if !grantsContentsWrite && effective.Scopes != nil {
		grantsContentsWrite = effective.Scopes["contents"] == "write"
	}
	if !grantsContentsWrite {
		return false
	}
	return !jobLooksLikeItWrites(j)
}

func jobLooksLikeItWrites(j Job) bool {
	for _, s := range j.Steps {
		text := strings.ToLower(s.Uses + " " + s.Run)
		for _, kw := range writeLikeKeywords {
			if strings.Contains(text, kw) {
				return true
			}
		}
	}
	return false
}
