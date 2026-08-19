package cicdscan

import "testing"

func TestSecretsRules(t *testing.T) {
	runRuleTable(t, []ruleTableCase{
		{
			name: "secret echoed fires", ruleID: "cicdscan.secrets.echoed", wantFire: true,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo ${{ secrets.API_KEY }}
`,
		},
		{
			name: "secret used in a curl header (not echoed) does not fire", ruleID: "cicdscan.secrets.echoed", wantFire: false,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: |
          curl -H "Authorization: Bearer ${{ secrets.API_KEY }}" https://api.example.com
`,
		},
		{
			name: "secrets: inherit on a reusable workflow call fires", ruleID: "cicdscan.secrets.inherit", wantFire: true,
			yaml: `
jobs:
  call:
    uses: ./.github/workflows/reusable.yml
    secrets: inherit
`,
		},
		{
			name: "explicit named secrets on a reusable workflow call does not fire", ruleID: "cicdscan.secrets.inherit", wantFire: false,
			yaml: `
jobs:
  call:
    uses: ./.github/workflows/reusable.yml
    secrets:
      token: ${{ secrets.TOKEN }}
`,
		},
		{
			name: "secret referenced in an if: condition fires", ruleID: "cicdscan.secrets.in-condition", wantFire: true,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - if: "secrets.API_KEY != ''"
        run: echo deploying
`,
		},
		{
			name: "if: condition with no secret reference does not fire", ruleID: "cicdscan.secrets.in-condition", wantFire: false,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - if: success()
        run: echo deploying
`,
		},
	})
}
