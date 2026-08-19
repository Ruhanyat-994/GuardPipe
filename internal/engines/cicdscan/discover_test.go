package cicdscan

import "testing"

func TestDiscoverWorkflows_FindsAWorkflowFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ".github/workflows/ci.yml", `
name: CI
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`)

	result, err := discoverWorkflows(dir)
	if err != nil {
		t.Fatalf("discoverWorkflows() error = %v", err)
	}
	if len(result.Workflows) != 1 {
		t.Fatalf("got %d workflows, want 1", len(result.Workflows))
	}
	wf := result.Workflows[0]
	if wf.Name != "CI" {
		t.Errorf("Name = %q, want CI", wf.Name)
	}
	if len(wf.Jobs) != 1 || wf.Jobs[0].ID != "build" {
		t.Errorf("Jobs = %+v, want one job with ID build", wf.Jobs)
	}
}

// TestDiscoverWorkflows_NoWorkflowsDirectory is the near-miss: a repository
// with no .github/workflows/ at all must not error — it's exactly the
// "nothing to discover" case Applicable's own Applicable=false path relies
// on, not a failure.
func TestDiscoverWorkflows_NoWorkflowsDirectory(t *testing.T) {
	dir := t.TempDir()
	result, err := discoverWorkflows(dir)
	if err != nil {
		t.Fatalf("discoverWorkflows() error = %v, want nil", err)
	}
	if len(result.Workflows) != 0 {
		t.Errorf("got %d workflows, want 0", len(result.Workflows))
	}
}

func TestDiscoverWorkflows_IgnoresNonYAMLFiles(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ".github/workflows/ci.yml", "on: push\njobs: {build: {runs-on: ubuntu-latest, steps: [{run: echo hi}]}}")
	writeTestFile(t, dir, ".github/workflows/README.md", "# not a workflow")

	result, err := discoverWorkflows(dir)
	if err != nil {
		t.Fatalf("discoverWorkflows() error = %v", err)
	}
	if len(result.Workflows) != 1 {
		t.Errorf("got %d workflows, want 1 (README.md must be ignored)", len(result.Workflows))
	}
}

func TestDiscoverWorkflows_GenuinelyMalformedYAMLIsAParseError(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, ".github/workflows/broken.yml", "on: [unterminated")

	result, err := discoverWorkflows(dir)
	if err != nil {
		t.Fatalf("discoverWorkflows() error = %v", err)
	}
	if len(result.ParseErrors) != 1 {
		t.Errorf("got %d parse errors, want 1", len(result.ParseErrors))
	}
	if len(result.RawContent) != 1 {
		t.Errorf("got %d raw content entries, want 1 — the AI pass still needs the raw text of a file that failed structured parsing", len(result.RawContent))
	}
}

func TestParseWorkflow_HandlesAllThreeOnShapes(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want []string
	}{
		{"bare string", "on: push\njobs: {}", []string{"push"}},
		{"list", "on: [push, pull_request]\njobs: {}", []string{"push", "pull_request"}},
		{"map", "on:\n  push: {}\n  pull_request_target: {}\njobs: {}", []string{"push", "pull_request_target"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wf, err := parseWorkflow([]byte(tc.yaml), "test.yml")
			if err != nil {
				t.Fatalf("parseWorkflow() error = %v", err)
			}
			if len(wf.Triggers) != len(tc.want) {
				t.Fatalf("Triggers = %v, want %v", wf.Triggers, tc.want)
			}
			for i, trig := range tc.want {
				if wf.Triggers[i] != trig {
					t.Errorf("Triggers[%d] = %q, want %q", i, wf.Triggers[i], trig)
				}
			}
		})
	}
}
