package cicdscan

import "testing"

func TestRunnerRules(t *testing.T) {
	runRuleTable(t, []ruleTableCase{
		{
			name: "self-hosted runner on a fork-triggerable workflow fires", ruleID: "cicdscan.runner.self-hosted-public", wantFire: true,
			yaml: `
on: pull_request
jobs:
  build:
    runs-on: self-hosted
    steps:
      - run: echo hi
`,
		},
		{
			name: "GitHub-hosted runner on a fork-triggerable workflow does not fire", ruleID: "cicdscan.runner.self-hosted-public", wantFire: false,
			yaml: `
on: pull_request
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`,
		},
		{
			// A real false positive found live-testing against
			// github.com/actions/checkout: runs-on: ${{ matrix.runs-on }}
			// with matrix values [ubuntu-latest, macos-latest,
			// windows-latest] was flagged as self-hosted, since the raw
			// expression string doesn't match githubHostedLabel — but it
			// isn't a self-hosted label at all, it's an unresolved
			// expression.
			name: "runs-on set via a matrix expression does not fire", ruleID: "cicdscan.runner.self-hosted-public", wantFire: false,
			yaml: `
on: pull_request
jobs:
  build:
    strategy:
      matrix:
        runs-on: [ubuntu-latest, macos-latest, windows-latest]
    runs-on: ${{ matrix.runs-on }}
    steps:
      - run: echo hi
`,
		},
		{
			name: "container image referenced by a mutable tag fires", ruleID: "cicdscan.runner.unpinned-image", wantFire: true,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    container:
      image: node:18
    steps:
      - run: echo hi
`,
		},
		{
			name: "container image pinned by digest does not fire", ruleID: "cicdscan.runner.unpinned-image", wantFire: false,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    container:
      image: node@sha256:abcdef1234567890123456789012345678901234567890123456789012345678
    steps:
      - run: echo hi
`,
		},
		{
			name: "artifact upload path matches a secret-shaped pattern fires", ruleID: "cicdscan.artifact.upload-sensitive", wantFire: true,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/upload-artifact@v4
        with:
          path: .env
`,
		},
		{
			name: "artifact upload path with no secret-shaped pattern does not fire", ruleID: "cicdscan.artifact.upload-sensitive", wantFire: false,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/upload-artifact@v4
        with:
          path: dist/
`,
		},
	})
}
