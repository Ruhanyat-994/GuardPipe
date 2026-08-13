//go:build integration

// Run with `go test ./internal/store/repo/... -tags=integration` against a
// real Docker daemon — see identity_integration_test.go's header for why.
package repo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/advisory"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
)

func exampleRule() advisory.Rule {
	return advisory.Rule{
		ID:              "codescan.injection.sql-string-concat",
		Engine:          domain.EngineCodeScan,
		Category:        "injection",
		Title:           "SQL built via string concatenation",
		Description:     "A SQL query is built by concatenating untrusted input.",
		Remediation:     "Use parameterised queries.",
		DefaultSeverity: domain.SeverityHigh,
		CWE:             []string{"CWE-89"},
		OWASP:           []string{"A03:2021"},
		References:      []string{"https://owasp.org/Top10/A03_2021-Injection/"},
		Tier:            domain.TierCore,
	}
}

func TestRuleRepo_UpsertAndGet_RoundTrip(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	rules := repo.NewRuleRepo(pool)

	require.NoError(t, rules.Upsert(ctx, exampleRule()))

	got, err := rules.GetByID(ctx, "codescan.injection.sql-string-concat")
	require.NoError(t, err)
	require.Equal(t, domain.EngineCodeScan, got.Engine)
	require.Equal(t, "injection", got.Category)
	require.Equal(t, []string{"CWE-89"}, got.CWE)
	require.True(t, got.Enabled, "a newly-inserted rule defaults to enabled")
	require.False(t, got.CreatedAt.IsZero())
}

func TestRuleRepo_GetByID_NotFound(t *testing.T) {
	pool := setupTestDB(t)
	rules := repo.NewRuleRepo(pool)

	_, err := rules.GetByID(context.Background(), "does.not.exist")
	require.Error(t, err)
}

func TestRuleRepo_SetEnabled_SurvivesReupsert(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	rules := repo.NewRuleRepo(pool)

	require.NoError(t, rules.Upsert(ctx, exampleRule()))
	_, err := rules.SetEnabled(ctx, "codescan.injection.sql-string-concat", false)
	require.NoError(t, err)

	// Re-syncing from the code registry (same rule, e.g. title tweak) must
	// not silently re-enable a rule an operator disabled.
	updated := exampleRule()
	updated.Title = "SQL built via string concatenation (updated)"
	require.NoError(t, rules.Upsert(ctx, updated))

	got, err := rules.GetByID(ctx, "codescan.injection.sql-string-concat")
	require.NoError(t, err)
	require.False(t, got.Enabled)
	require.Equal(t, "SQL built via string concatenation (updated)", got.Title)
}

func TestRuleRepo_SetEnabled_NotFound(t *testing.T) {
	pool := setupTestDB(t)
	rules := repo.NewRuleRepo(pool)

	_, err := rules.SetEnabled(context.Background(), "does.not.exist", true)
	require.Error(t, err)
}

func TestRuleRepo_List_FiltersAndPaginates(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	rules := repo.NewRuleRepo(pool)

	require.NoError(t, rules.Upsert(ctx, exampleRule()))
	dep := exampleRule()
	dep.ID = "depscan.vuln.known-cve"
	dep.Engine = domain.EngineDepScan
	dep.Category = "vuln"
	require.NoError(t, rules.Upsert(ctx, dep))

	codescan := domain.EngineCodeScan
	got, total, err := rules.List(ctx, advisory.RuleFilter{Engine: &codescan}, advisory.RulePage{Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "codescan.injection.sql-string-concat", got[0].ID)

	all, total, err := rules.List(ctx, advisory.RuleFilter{}, advisory.RulePage{Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, all, 2)
}
