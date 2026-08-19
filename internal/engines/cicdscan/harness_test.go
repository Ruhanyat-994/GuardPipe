package cicdscan

import (
	"testing"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// workflowFromYAML parses yamlDoc into a Workflow — the shared harness every
// rule-family test table below uses, so a true-positive/near-miss case is
// just a YAML string and an expected rule ID, not a filesystem fixture.
func workflowFromYAML(t *testing.T, yamlDoc string) Workflow {
	t.Helper()
	wf, err := parseWorkflow([]byte(yamlDoc), "test.yml")
	if err != nil {
		t.Fatalf("parseWorkflow() error = %v", err)
	}
	return wf
}

// evaluateAll runs every deterministic rule family against wf — what
// Engine.Run does per file, minus discovery/AI, so a rule test only has to
// assert "does ruleID appear," not thread findings through the full engine.
func evaluateAll(wf Workflow) []domain.Finding {
	scanID := uuid.New()
	var findings []domain.Finding
	findings = append(findings, evaluateSupplyChain(scanID, wf)...)
	findings = append(findings, evaluateTriggers(scanID, wf)...)
	findings = append(findings, evaluatePermissions(scanID, wf)...)
	findings = append(findings, evaluateSecrets(scanID, wf)...)
	findings = append(findings, evaluateRunner(scanID, wf)...)
	return findings
}

func hasRuleID(findings []domain.Finding, ruleID string) bool {
	for _, f := range findings {
		if f.RuleID == ruleID {
			return true
		}
	}
	return false
}

// ruleTableCase is one row of a true-positive/near-miss table — the
// standing shape documentation/15-testing-strategy.md's testing philosophy
// requires for every rule: "a true-positive test and a near-miss (must-not-fire)
// test in the same table."
type ruleTableCase struct {
	name     string
	yaml     string
	ruleID   string
	wantFire bool
}

func runRuleTable(t *testing.T, cases []ruleTableCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wf := workflowFromYAML(t, tc.yaml)
			got := hasRuleID(evaluateAll(wf), tc.ruleID)
			if got != tc.wantFire {
				t.Errorf("%s fired = %v, want %v", tc.ruleID, got, tc.wantFire)
			}
		})
	}
}
