// Package cicdscan is GuardPipe's GitHub Actions workflow security engine
// (documentation/05-module-specifications.md §10, BUILD_GUIDE.md Phase 10).
// Per ADR-0010 it is GuardPipe's own rule engine, not a wrapper — it reads
// `.github/workflows/*.yml` from the already-cloned workspace, evaluates 16
// Core deterministic rules, then (§10's own ordering: "deterministic rules
// first, AI is the supplement, not the foundation") runs one AI semantic
// pass per file through modules/ai, which may only add findings — it never
// removes or downgrades a rule finding.
package cicdscan

// Workflow is one parsed `.github/workflows/*.yml` file — a hand-parsed
// subset of the real GitHub Actions schema (the same reasoning k8sscan's
// own types.go gives for not pulling in a full upstream schema: this engine
// only ever reads the handful of fields its 16 rules care about).
//
// Line is always the file's first line (1) — workflow-level findings (a
// missing top-level permissions: block, a risky trigger combination) point
// at the top of the file, since there's no more specific "start" for a
// property of the whole document. Job.Line/Step.Line are each that
// job/step's own real start line in File, taken from the yaml.Node this was
// parsed from — the same document-level-not-field-level precision k8sscan's
// Manifest.DocLine already established: enough to land on the right job or
// step, not necessarily the exact offending sub-key within it.
type Workflow struct {
	File string // relative to the workspace root
	Name string
	Line int

	// Triggers is every top-level `on:` event name — "push", "pull_request",
	// "pull_request_target", "workflow_run", "workflow_dispatch", ... — in
	// whichever of the three shapes on: was written (a bare string, a list,
	// or a map), collected uniformly into this one slice.
	Triggers []string
	// WorkflowRunDownloadsArtifact is set when a workflow_run-triggered
	// workflow also has a step that looks like it downloads an artifact
	// from the triggering run (cicdscan.trigger.workflow-run-untrusted) —
	// a raw-text heuristic (searching for actions/download-artifact plus a
	// github.event.workflow_run reference anywhere in the file), not a full
	// dataflow analysis of which specific artifact ends up where.
	WorkflowRunDownloadsArtifact bool

	Permissions     Permissions
	PermissionsLine int // 0 when there is no top-level permissions: block at all

	Jobs []Job
}

// Permissions covers both shapes `permissions:` can take: a bare scalar
// (`read-all` | `write-all`) or a map of scope -> level. Present is false
// when the key is absent entirely, distinct from an explicit empty map.
type Permissions struct {
	Present  bool
	WriteAll bool
	ReadAll  bool
	Scopes   map[string]string // e.g. {"contents": "write", "issues": "read"}
}

// WriteScopes returns every scope this Permissions block grants write
// access to, "write-all" expanded to a single synthetic "all" entry so
// callers don't need to special-case the scalar form.
func (p Permissions) WriteScopes() []string {
	if p.WriteAll {
		return []string{"all"}
	}
	var out []string
	for scope, level := range p.Scopes {
		if level == "write" {
			out = append(out, scope)
		}
	}
	return out
}

// Job is one entry under `jobs:`.
type Job struct {
	ID   string
	Line int

	RunsOn []string // one or more labels/expressions — matrix `runs-on` collapses to the raw list as written

	// Container is the job-level `container:` image reference, "" if the
	// job has none — either the bare-scalar form (`container: node:18`) or
	// the mapping form's `image:` key.
	Container string

	Permissions     Permissions
	PermissionsLine int

	// Uses/SecretsInherit describe this job as a *caller* of a reusable
	// workflow (`uses: ./.github/workflows/build.yml`) — Uses is "" for an
	// ordinary job.
	Uses           string
	SecretsInherit bool

	Steps []Step
}

// Step is one entry under a job's `steps:`.
type Step struct {
	Line int
	Name string
	Uses string // "" if this step is a `run:` step instead
	Run  string // "" if this step is a `uses:` step instead
	If   string // the raw `if:` expression, "" if absent
	// With holds a `uses:` step's scalar `with:` values only (e.g. the
	// canonical `with: ref: ${{ github.event.pull_request.head.sha }}` on a
	// checkout step) — a nested/list with: value is skipped, the same
	// "only the slice of the schema the rules actually need" scoping every
	// other hand-written type in this engine already uses.
	With map[string]string
}
