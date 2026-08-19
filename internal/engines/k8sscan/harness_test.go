package k8sscan

import (
	"testing"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// graphFromYAML parses yamlDoc (one or more `---`-separated Kubernetes
// documents) into a ResourceGraph — the shared harness every rule-family
// test table below uses, so a true-positive/near-miss case is just a YAML
// string and an expected rule ID, not a filesystem fixture.
func graphFromYAML(t *testing.T, yamlDoc string) *ResourceGraph {
	t.Helper()
	manifests, templated, err := parseDocuments([]byte(yamlDoc), "test.yaml")
	if err != nil {
		t.Fatalf("parseDocuments() error = %v", err)
	}
	if templated {
		t.Fatal("parseDocuments() reported templated content for a literal test fixture")
	}
	graph := newResourceGraph()
	for _, m := range manifests {
		graph.add(m)
	}
	return graph
}

// evaluateAll runs every rule family against graph — what Engine.Run does,
// minus discovery/Helm, so a rule test only has to assert "does ruleID
// appear," not thread findings through the full engine.
func evaluateAll(graph *ResourceGraph) []domain.Finding {
	scanID := uuid.New()
	var findings []domain.Finding
	findings = append(findings, evaluateRBAC(graph, scanID)...)
	findings = append(findings, evaluateWorkload(graph, scanID)...)
	findings = append(findings, evaluateNetwork(graph, scanID)...)
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
			graph := graphFromYAML(t, tc.yaml)
			got := hasRuleID(evaluateAll(graph), tc.ruleID)
			if got != tc.wantFire {
				t.Errorf("%s fired = %v, want %v", tc.ruleID, got, tc.wantFire)
			}
		})
	}
}
