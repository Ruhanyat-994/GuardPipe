package cicdscan

import "testing"

func TestPermissionsRules(t *testing.T) {
	runRuleTable(t, []ruleTableCase{
		{
			name: "no top-level permissions block fires", ruleID: "cicdscan.permissions.missing-block", wantFire: true,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`,
		},
		{
			name: "explicit top-level permissions block does not fire", ruleID: "cicdscan.permissions.missing-block", wantFire: false,
			yaml: `
permissions:
  contents: read
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`,
		},
		{
			name: "permissions write-all fires", ruleID: "cicdscan.permissions.write-all", wantFire: true,
			yaml: `
permissions: write-all
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`,
		},
		{
			name: "scoped read-only permissions do not fire write-all", ruleID: "cicdscan.permissions.write-all", wantFire: false,
			yaml: `
permissions:
  contents: read
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`,
		},
		{
			name: "contents write with no step that looks like it writes fires", ruleID: "cicdscan.permissions.excessive-token", wantFire: true,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
      - run: go test ./...
`,
		},
		{
			name: "contents write with a step that publishes a release does not fire", ruleID: "cicdscan.permissions.excessive-token", wantFire: false,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
      - uses: softprops/action-gh-release@v1
`,
		},
	})
}
