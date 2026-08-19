package cicdscan

import "testing"

func TestTriggerRules(t *testing.T) {
	runRuleTable(t, []ruleTableCase{
		{
			name: "pull_request_target checks out the PR's own head fires", ruleID: "cicdscan.trigger.pull-request-target-checkout", wantFire: true,
			yaml: `
on: pull_request_target
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.event.pull_request.head.sha }}
`,
		},
		{
			name: "pull_request_target with a plain checkout (base ref) does not fire", ruleID: "cicdscan.trigger.pull-request-target-checkout", wantFire: false,
			yaml: `
on: pull_request_target
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`,
		},
		{
			name: "workflow_run downloads an artifact from the triggering run fires", ruleID: "cicdscan.trigger.workflow-run-untrusted", wantFire: true,
			yaml: `
on:
  workflow_run:
    workflows: ["CI"]
    types: [completed]
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v4
        with:
          run-id: ${{ github.event.workflow_run.id }}
`,
		},
		{
			name: "workflow_run with no artifact download does not fire", ruleID: "cicdscan.trigger.workflow-run-untrusted", wantFire: false,
			yaml: `
on:
  workflow_run:
    workflows: ["CI"]
    types: [completed]
jobs:
  notify:
    runs-on: ubuntu-latest
    steps:
      - run: echo "CI finished"
`,
		},
		{
			name: "github.event.* interpolated directly into run: fires", ruleID: "cicdscan.injection.script-injection", wantFire: true,
			yaml: `
on: issues
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo "${{ github.event.issue.title }}"
`,
		},
		{
			name: "github.event.* passed through env: instead does not fire", ruleID: "cicdscan.injection.script-injection", wantFire: false,
			yaml: `
on: issues
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - env:
          TITLE: ${{ github.event.issue.title }}
        run: echo "$TITLE"
`,
		},
	})
}
