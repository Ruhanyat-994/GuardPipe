package cicdscan

import "testing"

func TestSupplyChainRules(t *testing.T) {
	runRuleTable(t, []ruleTableCase{
		{
			name: "action pinned by mutable tag fires", ruleID: "cicdscan.supply-chain.unpinned-action", wantFire: true,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`,
		},
		{
			name: "action pinned by full commit SHA does not fire", ruleID: "cicdscan.supply-chain.unpinned-action", wantFire: false,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@1caa1c1b8e2c0349eda06d7b93b0d5654a1e1e15
`,
		},
		{
			name: "action from an unrecognised publisher fires", ruleID: "cicdscan.supply-chain.unverified-action", wantFire: true,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: some-random-org/some-action@v1
`,
		},
		{
			name: "action from a trusted publisher does not fire", ruleID: "cicdscan.supply-chain.unverified-action", wantFire: false,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
`,
		},
		{
			name: "curl piped into bash fires", ruleID: "cicdscan.supply-chain.curl-pipe-shell", wantFire: true,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: curl https://example.com/install.sh | bash
`,
		},
		{
			name: "curl downloaded to a file, not piped, does not fire", ruleID: "cicdscan.supply-chain.curl-pipe-shell", wantFire: false,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: curl -o install.sh https://example.com/install.sh
`,
		},
		{
			name: "pip install with no version pin fires", ruleID: "cicdscan.supply-chain.unpinned-install", wantFire: true,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: pip install requests
`,
		},
		{
			name: "pip install pinned to an exact version does not fire", ruleID: "cicdscan.supply-chain.unpinned-install", wantFire: false,
			yaml: `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: pip install requests==2.31.0
`,
		},
	})
}
