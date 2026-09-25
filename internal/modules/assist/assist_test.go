package assist

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/billing"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

type fakeRepo struct{ fc *FindingContext }

func (f fakeRepo) GetFindingContext(context.Context, uuid.UUID) (*FindingContext, error) {
	if f.fc == nil {
		return nil, apperrors.NotFound("finding.not_found", "nope")
	}
	cp := *f.fc
	return &cp, nil
}

type fakeAI struct {
	calls  []ai.RunInput
	result *ai.RunResult
	err    error
}

func (f *fakeAI) Run(_ context.Context, in ai.RunInput, _ func(domain.Finding)) (*ai.RunResult, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	switch in.PromptID {
	case ai.PromptExplainFinding:
		return &ai.RunResult{Value: ai.ExplainFindingResponse{What: "w", WhyItMatters: "y", HowExploited: "h", Confidence: "high"}}, nil
	case ai.PromptRemediateFinding:
		return &ai.RunResult{Value: ai.RemediateFindingResponse{Summary: "s", Steps: []ai.RemediationStep{{Title: "t", Detail: "d"}}, Verification: "v", Confidence: "high"}}, nil
	}
	return &ai.RunResult{Value: ai.GeneratePatchResponse{Patch: "--- a/x\n+++ b/x\n", Explanation: "e", Confidence: "medium", Caveats: []string{}}}, nil
}

type fakeTokens struct {
	balance int64
	paid    bool
	charged []billing.AIAction
}

func (f *fakeTokens) AIQuote(_ context.Context, _, _ uuid.UUID, a billing.AIAction) (int64, bool, int64, error) {
	return billing.AIPrice[a], f.paid, f.balance, nil
}

func (f *fakeTokens) ChargeAI(_ context.Context, _, _, _, _ uuid.UUID, a billing.AIAction) (int64, error) {
	f.charged = append(f.charged, a)
	return billing.AIPrice[a], nil
}

type fakeClone struct{}

func (fakeClone) GetCloneInfo(context.Context, uuid.UUID) (string, string, string, error) {
	return "https://github.com/acme/api", "main", "tok", nil
}

type fakeSource struct {
	content string
	refs    []string
}

func (f *fakeSource) FetchFile(_ context.Context, _, ref, _, _ string) (string, error) {
	f.refs = append(f.refs, ref)
	return f.content, nil
}

type harness struct {
	svc    *Service
	ai     *fakeAI
	tokens *fakeTokens
	source *fakeSource
	fc     *FindingContext
	actor  domain.Actor
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	org := uuid.New()
	sha := "abc123"
	fc := &FindingContext{
		OrgID: org, ProjectID: uuid.New(), CommitSHA: &sha,
		Finding: domain.Finding{
			ID: uuid.New(), ScanID: uuid.New(), Engine: domain.EngineCodeScan,
			RuleID: "codescan.injection.sql-string-concat", Title: "SQL built by concatenation",
			Severity: domain.SeverityHigh, CWE: []string{"CWE-89"},
			Location: domain.Location{Type: domain.LocationTypeFile, Path: "db/users.go", LineStart: 42},
			Evidence: []domain.Evidence{{Kind: domain.EvidenceKindCodeSnippet, Value: `q := "SELECT " + id`, LineStart: 42}},
		},
	}
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "code line"
	}
	h := &harness{
		ai: &fakeAI{}, tokens: &fakeTokens{balance: 10_000},
		source: &fakeSource{content: strings.Join(lines, "\n")}, fc: fc,
		actor: domain.Actor{UserID: uuid.New(), OrgID: org, Role: domain.RoleMember},
	}
	h.svc = NewService(fakeRepo{fc: fc}, h.ai, h.tokens, fakeClone{}, h.source, nil)
	return h
}

func requireKind(t *testing.T, err error, kind apperrors.Kind) {
	t.Helper()
	var appErr *apperrors.Error
	require.True(t, errors.As(err, &appErr), "want app error, got %v", err)
	require.Equal(t, kind, appErr.Kind)
}

func TestExplain_ChargesOnceAndUsesEvidenceNotSource(t *testing.T) {
	h := newHarness(t)
	r, err := h.svc.Run(context.Background(), h.actor, h.fc.Finding.ID, billing.AIExplain)
	require.NoError(t, err)
	require.NotNil(t, r.Explain)
	require.Equal(t, int64(150), r.TokensCharged)
	require.Equal(t, []billing.AIAction{billing.AIExplain}, h.tokens.charged)
	require.Empty(t, h.source.refs, "explain never fetches source")
	require.Len(t, h.ai.calls[0].Untrusted, 1)
	require.Contains(t, h.ai.calls[0].Untrusted[0].Content, "SELECT")
	require.Equal(t, "CWE-89", h.ai.calls[0].Vars["cwe"])
}

func TestFix_UsesNumberedSourceAtScannedCommit(t *testing.T) {
	h := newHarness(t)
	r, err := h.svc.Run(context.Background(), h.actor, h.fc.Finding.ID, billing.AIFix)
	require.NoError(t, err)
	require.NotNil(t, r.Fix)
	require.True(t, r.SourceUsed)
	require.Equal(t, []string{"abc123"}, h.source.refs, "fetched at the scanned commit, not the branch")
	block := h.ai.calls[0].Untrusted
	require.Len(t, block, 1, "source replaces the evidence snippet")
	require.True(t, strings.HasPrefix(block[0].Content, "2 | "), "excerpt starts 40 lines above line 42")
	require.Contains(t, block[0].Content, "82 | ")
	require.NotContains(t, block[0].Content, "83 | ")
	require.Equal(t, "Go", h.ai.calls[0].Vars["language"])
}

// Near-miss: a secret finding's file holds the secret itself — it must
// never be fetched or sent to the model.
func TestRemediate_SecretFinding_NeverFetchesSource(t *testing.T) {
	h := newHarness(t)
	h.fc.Finding.RuleID = "depscan.secrets.aws-access-key"
	h.fc.Finding.Evidence = []domain.Evidence{{Kind: domain.EvidenceKindCodeSnippet, Value: "AKIA****", Redacted: true}}
	r, err := h.svc.Run(context.Background(), h.actor, h.fc.Finding.ID, billing.AIRemediate)
	require.NoError(t, err)
	require.False(t, r.SourceUsed)
	require.Empty(t, h.source.refs)
	require.Equal(t, "AKIA****", h.ai.calls[0].Untrusted[0].Content)
}

func TestRun_OtherOrgOrOtherProject_NotFound(t *testing.T) {
	h := newHarness(t)
	other := h.actor
	other.OrgID = uuid.New()
	_, err := h.svc.Run(context.Background(), other, h.fc.Finding.ID, billing.AIExplain)
	requireKind(t, err, apperrors.KindNotFound)

	scoped := h.actor
	p := uuid.New()
	scoped.ProjectID = &p
	_, err = h.svc.Run(context.Background(), scoped, h.fc.Finding.ID, billing.AIExplain)
	requireKind(t, err, apperrors.KindNotFound)
	require.Empty(t, h.ai.calls)
}

func TestRun_NotEnoughTokens_NoModelCallNoCharge(t *testing.T) {
	h := newHarness(t)
	h.tokens.balance = 100
	_, err := h.svc.Run(context.Background(), h.actor, h.fc.Finding.ID, billing.AIFix)
	requireKind(t, err, apperrors.KindPaymentRequired)
	require.Empty(t, h.ai.calls)
	require.Empty(t, h.tokens.charged)
}

func TestRun_AlreadyPaid_IsFree(t *testing.T) {
	h := newHarness(t)
	h.tokens.paid = true
	h.tokens.balance = 0
	r, err := h.svc.Run(context.Background(), h.actor, h.fc.Finding.ID, billing.AIRemediate)
	require.NoError(t, err)
	require.True(t, r.AlreadyPaid)
	require.Zero(t, r.TokensCharged)
	require.Empty(t, h.tokens.charged)
}

func TestRun_ModelFailsOrDiscards_NothingCharged(t *testing.T) {
	h := newHarness(t)
	h.ai.err = errors.New("gemini 503")
	_, err := h.svc.Run(context.Background(), h.actor, h.fc.Finding.ID, billing.AIExplain)
	requireKind(t, err, apperrors.KindExternal)

	h.ai.err = nil
	h.ai.result = &ai.RunResult{Discarded: true}
	_, err = h.svc.Run(context.Background(), h.actor, h.fc.Finding.ID, billing.AIExplain)
	requireKind(t, err, apperrors.KindUnprocessable)
	require.Empty(t, h.tokens.charged)
}

func TestRun_AIDisabled(t *testing.T) {
	h := newHarness(t)
	svc := NewService(fakeRepo{fc: h.fc}, nil, h.tokens, nil, nil, nil)
	_, err := svc.Run(context.Background(), h.actor, h.fc.Finding.ID, billing.AIExplain)
	requireKind(t, err, apperrors.KindUnprocessable)
}

func TestRun_BillingOff_Free(t *testing.T) {
	h := newHarness(t)
	svc := NewService(fakeRepo{fc: h.fc}, h.ai, nil, nil, nil, nil)
	r, err := svc.Run(context.Background(), h.actor, h.fc.Finding.ID, billing.AIExplain)
	require.NoError(t, err)
	require.Zero(t, r.TokensCharged)
}

func TestNumberedExcerpt(t *testing.T) {
	content := "a\nb\nc\nd\ne"
	require.Equal(t, "2 | b\n3 | c\n4 | d\n", numberedExcerpt(content, 3, 3, 1))
	require.Equal(t, "1 | a\n2 | b\n", numberedExcerpt(content, 0, 0, 1), "no line info: the top of the file")
	require.Equal(t, "4 | d\n5 | e\n", numberedExcerpt(content, 5, 5, 1), "clamped at the end")
}

// Near-miss: a dependency finding has no source line to patch — Fix is
// refused before any model call or charge.
func TestFix_DependencyFinding_NotPatchable(t *testing.T) {
	h := newHarness(t)
	h.fc.Finding.Location = domain.Location{Type: domain.LocationTypeDependency, Package: "lodash", Version: "4.17.15", Ecosystem: "npm"}
	_, err := h.svc.Run(context.Background(), h.actor, h.fc.Finding.ID, billing.AIFix)
	requireKind(t, err, apperrors.KindUnprocessable)
	require.Empty(t, h.ai.calls)
	require.Empty(t, h.tokens.charged)
}

func TestRun_ProviderOutOfQuota_ClearRateLimit(t *testing.T) {
	h := newHarness(t)
	h.ai.err = fmt.Errorf("ai: %w", apperrors.External("gemini.unavailable", "every key is out of quota", errors.New("429")))
	_, err := h.svc.Run(context.Background(), h.actor, h.fc.Finding.ID, billing.AIExplain)
	requireKind(t, err, apperrors.KindRateLimited)
	require.Empty(t, h.tokens.charged)
}
