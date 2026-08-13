package advisory_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/advisory"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// fakeRuleRepo is a hand-written in-memory RuleRepository fake — no mocking
// framework per the project's testing philosophy.
type fakeRuleRepo struct {
	mu    sync.Mutex
	rules map[string]advisory.Rule
}

func newFakeRuleRepo() *fakeRuleRepo {
	return &fakeRuleRepo{rules: make(map[string]advisory.Rule)}
}

func (f *fakeRuleRepo) Upsert(_ context.Context, r advisory.Rule) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.rules[r.ID]; ok {
		r.Enabled = existing.Enabled // Upsert never touches Enabled on conflict
		r.CreatedAt = existing.CreatedAt
	} else {
		r.Enabled = true
		r.CreatedAt = time.Now()
	}
	r.UpdatedAt = time.Now()
	f.rules[r.ID] = r
	return nil
}

func (f *fakeRuleRepo) List(_ context.Context, filter advisory.RuleFilter, _ advisory.RulePage) ([]advisory.Rule, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []advisory.Rule
	for _, r := range f.rules {
		if filter.Engine != nil && r.Engine != *filter.Engine {
			continue
		}
		if filter.Tier != nil && r.Tier != *filter.Tier {
			continue
		}
		if filter.Severity != nil && r.DefaultSeverity != *filter.Severity {
			continue
		}
		out = append(out, r)
	}
	return out, len(out), nil
}

func (f *fakeRuleRepo) GetByID(_ context.Context, id string) (*advisory.Rule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rules[id]
	if !ok {
		return nil, apperrors.NotFound("rule.not_found", "rule not found")
	}
	return &r, nil
}

func (f *fakeRuleRepo) SetEnabled(_ context.Context, id string, enabled bool) (*advisory.Rule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.rules[id]
	if !ok {
		return nil, apperrors.NotFound("rule.not_found", "rule not found")
	}
	r.Enabled = enabled
	r.UpdatedAt = time.Now()
	f.rules[id] = r
	return &r, nil
}

func exampleRuleMeta() domain.RuleMeta {
	return domain.RuleMeta{
		ID:          "codescan.injection.sql-string-concat",
		Title:       "SQL built via string concatenation",
		Description: "A SQL query is built by concatenating untrusted input.",
		Severity:    domain.SeverityHigh,
		Confidence:  domain.ConfidenceHigh,
		CWE:         []string{"CWE-89"},
		Remediation: "Use parameterised queries.",
		Tier:        domain.TierCore,
	}
}

func TestSyncRules_UpsertsRegisteredRules(t *testing.T) {
	repo := newFakeRuleRepo()
	registry := advisory.NewRuleRegistry()
	registry.Register(exampleRuleMeta())
	svc := advisory.NewService(&fakeOSVClient{}, newFakeCache(), time.Hour, repo, registry, nil)

	require.NoError(t, svc.SyncRules(context.Background()))

	r, err := svc.GetRule(context.Background(), "codescan.injection.sql-string-concat")
	require.NoError(t, err)
	require.Equal(t, domain.EngineCodeScan, r.Engine)
	require.Equal(t, "injection", r.Category)
	require.Equal(t, domain.SeverityHigh, r.DefaultSeverity)
	require.True(t, r.Enabled)
}

func TestSyncRules_EmptyRegistry_UpsertsNothing(t *testing.T) {
	repo := newFakeRuleRepo()
	registry := advisory.NewRuleRegistry()
	svc := advisory.NewService(&fakeOSVClient{}, newFakeCache(), time.Hour, repo, registry, nil)

	require.NoError(t, svc.SyncRules(context.Background()))

	rules, total, err := svc.ListRules(context.Background(), advisory.RuleFilter{}, advisory.RulePage{})
	require.NoError(t, err)
	require.Equal(t, 0, total)
	require.Empty(t, rules)
}

func TestSyncRules_PreservesOperatorDisabledStateAcrossResync(t *testing.T) {
	repo := newFakeRuleRepo()
	registry := advisory.NewRuleRegistry()
	registry.Register(exampleRuleMeta())
	svc := advisory.NewService(&fakeOSVClient{}, newFakeCache(), time.Hour, repo, registry, nil)

	require.NoError(t, svc.SyncRules(context.Background()))
	_, err := svc.SetRuleEnabled(context.Background(), "codescan.injection.sql-string-concat", false)
	require.NoError(t, err)

	// Simulate a restart re-syncing the same registered rule.
	require.NoError(t, svc.SyncRules(context.Background()))

	r, err := svc.GetRule(context.Background(), "codescan.injection.sql-string-concat")
	require.NoError(t, err)
	require.False(t, r.Enabled, "a re-sync must not silently re-enable an operator-disabled rule")
}

func TestSyncRules_MalformedRuleID(t *testing.T) {
	repo := newFakeRuleRepo()
	registry := advisory.NewRuleRegistry()
	bad := exampleRuleMeta()
	bad.ID = "not-namespaced"
	registry.Register(bad)
	svc := advisory.NewService(&fakeOSVClient{}, newFakeCache(), time.Hour, repo, registry, nil)

	err := svc.SyncRules(context.Background())
	require.Error(t, err)
}

func TestGetRule_NotFound(t *testing.T) {
	repo := newFakeRuleRepo()
	svc := advisory.NewService(&fakeOSVClient{}, newFakeCache(), time.Hour, repo, advisory.NewRuleRegistry(), nil)

	_, err := svc.GetRule(context.Background(), "codescan.injection.does-not-exist")
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindNotFound, appErr.Kind)
}

func TestSetRuleEnabled_NotFound(t *testing.T) {
	repo := newFakeRuleRepo()
	svc := advisory.NewService(&fakeOSVClient{}, newFakeCache(), time.Hour, repo, advisory.NewRuleRegistry(), nil)

	_, err := svc.SetRuleEnabled(context.Background(), "codescan.injection.does-not-exist", false)
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindNotFound, appErr.Kind)
}

func TestListRules_FiltersByEngine(t *testing.T) {
	repo := newFakeRuleRepo()
	registry := advisory.NewRuleRegistry()
	registry.Register(exampleRuleMeta())
	depMeta := exampleRuleMeta()
	depMeta.ID = "depscan.vuln.known-cve"
	registry.Register(depMeta)
	svc := advisory.NewService(&fakeOSVClient{}, newFakeCache(), time.Hour, repo, registry, nil)
	require.NoError(t, svc.SyncRules(context.Background()))

	codescan := domain.EngineCodeScan
	rules, total, err := svc.ListRules(context.Background(), advisory.RuleFilter{Engine: &codescan}, advisory.RulePage{})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Equal(t, "codescan.injection.sql-string-concat", rules[0].ID)
}

func TestParseRuleID(t *testing.T) {
	engine, category, err := advisory.ParseRuleID("codescan.injection.sql-string-concat")
	require.NoError(t, err)
	require.Equal(t, domain.EngineCodeScan, engine)
	require.Equal(t, "injection", category)

	_, _, err = advisory.ParseRuleID("bad-id")
	require.Error(t, err)

	_, _, err = advisory.ParseRuleID("unknownengine.cat.rule")
	require.Error(t, err)
}
