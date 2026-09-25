package livescan_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/github"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/livescan"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/crypto"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

type harness struct {
	svc      livescan.Service
	repo     *fakeRepo
	hooks    *fakeHooks
	projects *fakeProjects
	scans    *fakeScans
	coord    *fakeCoord
	audit    *fakeAudit
	actor    domain.Actor
}

func newHarness(t *testing.T, maxPerHour int) *harness {
	t.Helper()
	orgID := uuid.New()
	h := &harness{
		repo:  newFakeRepo(),
		hooks: &fakeHooks{},
		projects: &fakeProjects{
			projectID: uuid.New(), orgID: orgID, repoURL: "https://github.com/acme/payments-api",
			defaultBranch: "main", token: "ghp_hookscope",
		},
		scans: &fakeScans{},
		coord: newFakeCoord(),
		audit: &fakeAudit{},
		actor: domain.Actor{UserID: uuid.New(), OrgID: orgID, Role: domain.RoleAdmin},
	}
	h.svc = livescan.NewService(livescan.Config{
		PublicBaseURL: "https://guardpipe.example/", EncryptionKey: testKey,
		MaxScansPerProjectPerHour: maxPerHour, Debounce: 30 * time.Second,
	}, h.repo, h.hooks, h.projects, h.scans, h.coord, h.audit, nil)
	return h
}

func (h *harness) enable(t *testing.T, engines ...domain.EngineID) *livescan.Webhook {
	t.Helper()
	if len(engines) == 0 {
		engines = []domain.EngineID{domain.EngineCodeScan, domain.EngineDepScan}
	}
	w, err := h.svc.Enable(context.Background(), h.actor, h.projects.projectID, livescan.EnableInput{Engines: engines, Confirmed: true})
	require.NoError(t, err)
	return w
}

// secret recovers the plaintext signing secret from the stored row, the way
// GitHub (which was handed it at CreateHook time) would sign with it.
func (h *harness) secret(t *testing.T) []byte {
	t.Helper()
	w := h.repo.only()
	s, err := crypto.Decrypt(testKey, w.SecretCiphertext, w.SecretNonce)
	require.NoError(t, err)
	return s
}

func (h *harness) deliver(t *testing.T, event, deliveryID string, body []byte) error {
	t.Helper()
	w := h.repo.only()
	return h.svc.Receive(context.Background(), livescan.Delivery{
		WebhookID: w.ID, Event: event, DeliveryID: deliveryID, Body: body,
		Signature: github.Sign(h.secret(t), body),
	})
}

// drain runs the worker over everything queued, then fires any debounced
// scans as if the window had ended.
func (h *harness) drain(t *testing.T) {
	t.Helper()
	for {
		raw, _ := h.coord.PopEvent(context.Background(), 0)
		if raw == nil {
			break
		}
		require.NoError(t, h.svc.HandleQueuedEvent(context.Background(), raw))
	}
	require.NoError(t, h.svc.FireDue(context.Background(), time.Now().Add(time.Hour)))
}

func pushBody(repo, branch, sha, sender string) []byte {
	return []byte(fmt.Sprintf(`{"ref":"refs/heads/%s","after":"%s","deleted":false,"repository":{"full_name":"%s"},"sender":{"login":"%s"}}`,
		branch, sha, repo, sender))
}

func prBody(action string, number int, headRepo, baseRepo, headRef, baseRef string) []byte {
	return prBodyAt("cafe", action, number, headRepo, baseRepo, headRef, baseRef)
}

// prBodyAt is prBody with an explicit head commit — each real "synchronize"
// event carries a new one, and the same commit is only ever scanned once.
func prBodyAt(sha, action string, number int, headRepo, baseRepo, headRef, baseRef string) []byte {
	return []byte(fmt.Sprintf(`{"action":"%s","number":%d,"pull_request":{"head":{"ref":"%s","sha":"%s","repo":{"full_name":"%s"}},"base":{"ref":"%s"}},"repository":{"full_name":"%s"},"sender":{"login":"octocat"}}`,
		action, number, headRef, sha, headRepo, baseRef, baseRepo))
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, code, appErr.Code)
}

// --- Enable / Get / Disable ---

func TestEnable_RegistersHookAndStoresEncryptedSecret(t *testing.T) {
	h := newHarness(t, 10)
	w := h.enable(t)

	require.Len(t, h.hooks.created, 1)
	c := h.hooks.created[0]
	require.Equal(t, "acme", c.owner)
	require.Equal(t, "payments-api", c.name)
	require.Equal(t, "ghp_hookscope", c.token)
	require.Equal(t, "https://guardpipe.example/api/v1/webhooks/github/"+w.ID.String(), c.url)
	require.Equal(t, []string{"push", "pull_request"}, c.events)

	stored := h.repo.only()
	require.EqualValues(t, 777, stored.GitHubHookID)
	require.Equal(t, []string{"main"}, stored.WatchedBranches, "no branches given means the default branch")
	require.Equal(t, h.actor.UserID, *stored.EnabledBy)
	require.NotContains(t, string(stored.SecretCiphertext), c.secret, "secret must be stored encrypted")
	require.Equal(t, c.secret, string(h.secret(t)))
	require.Contains(t, h.audit.actions(), "webhook.enabled")
}

func TestEnable_RequiresConfirmation(t *testing.T) {
	h := newHarness(t, 10)
	_, err := h.svc.Enable(context.Background(), h.actor, h.projects.projectID, livescan.EnableInput{
		Engines: []domain.EngineID{domain.EngineCodeScan}, Confirmed: false,
	})
	requireCode(t, err, "livescan.confirmation_required")
	require.Empty(t, h.hooks.created)
}

func TestEnable_RejectsPentest(t *testing.T) {
	h := newHarness(t, 10)
	_, err := h.svc.Enable(context.Background(), h.actor, h.projects.projectID, livescan.EnableInput{
		Engines: []domain.EngineID{domain.EngineCodeScan, domain.EnginePentest}, Confirmed: true,
	})
	requireCode(t, err, "livescan.pentest_not_allowed")
	require.Empty(t, h.hooks.created)
}

func TestEnable_RejectsUnknownEngineAndBadBranch(t *testing.T) {
	h := newHarness(t, 10)
	_, err := h.svc.Enable(context.Background(), h.actor, h.projects.projectID, livescan.EnableInput{
		Engines: []domain.EngineID{"madeup"}, Confirmed: true,
	})
	requireCode(t, err, "livescan.invalid_input")

	_, err = h.svc.Enable(context.Background(), h.actor, h.projects.projectID, livescan.EnableInput{
		Engines: []domain.EngineID{domain.EngineCodeScan}, WatchedBranches: []string{"refs/heads/main"}, Confirmed: true,
	})
	requireCode(t, err, "livescan.invalid_branch")
}

func TestEnable_NeedsRepositoryAndToken(t *testing.T) {
	h := newHarness(t, 10)
	h.projects.token = ""
	_, err := h.svc.Enable(context.Background(), h.actor, h.projects.projectID, livescan.EnableInput{
		Engines: []domain.EngineID{domain.EngineCodeScan}, Confirmed: true,
	})
	requireCode(t, err, "livescan.credential_required")

	h.projects.noRepository = true
	_, err = h.svc.Enable(context.Background(), h.actor, h.projects.projectID, livescan.EnableInput{
		Engines: []domain.EngineID{domain.EngineCodeScan}, Confirmed: true,
	})
	requireCode(t, err, "livescan.no_repository")
}

func TestEnable_CrossOrgIs404(t *testing.T) {
	h := newHarness(t, 10)
	stranger := domain.Actor{UserID: uuid.New(), OrgID: uuid.New(), Role: domain.RoleAdmin}
	_, err := h.svc.Enable(context.Background(), stranger, h.projects.projectID, livescan.EnableInput{
		Engines: []domain.EngineID{domain.EngineCodeScan}, Confirmed: true,
	})
	requireCode(t, err, "project.not_found")

	h.enable(t)
	_, err = h.svc.Get(context.Background(), stranger, h.projects.projectID)
	requireCode(t, err, "project.not_found")
	_, err = h.svc.Disable(context.Background(), stranger, h.projects.projectID)
	requireCode(t, err, "project.not_found")
}

func TestEnable_SaveFailureRemovesTheHookAgain(t *testing.T) {
	h := newHarness(t, 10)
	h.repo.createErr = errors.New("db down")
	_, err := h.svc.Enable(context.Background(), h.actor, h.projects.projectID, livescan.EnableInput{
		Engines: []domain.EngineID{domain.EngineCodeScan}, Confirmed: true,
	})
	require.Error(t, err)
	require.Equal(t, []int64{777}, h.hooks.deleted)
}

func TestEnable_AgainUpdatesSettingsAndResumesWithoutANewHook(t *testing.T) {
	h := newHarness(t, 10)
	w := h.enable(t)
	require.NoError(t, h.repo.Pause(context.Background(), w.ID, "too many", time.Now()))

	other := domain.Actor{UserID: uuid.New(), OrgID: h.actor.OrgID, Role: domain.RoleAdmin}
	updated, err := h.svc.Enable(context.Background(), other, h.projects.projectID, livescan.EnableInput{
		Engines: []domain.EngineID{domain.EngineK8sScan}, WatchedBranches: []string{"main", "develop"}, Confirmed: true,
	})
	require.NoError(t, err)
	require.Len(t, h.hooks.created, 1, "settings change must not register a second hook")
	require.Equal(t, w.ID, updated.ID)
	require.Nil(t, updated.PausedReason)
	require.Equal(t, []domain.EngineID{domain.EngineK8sScan}, updated.Engines)
	require.Equal(t, []string{"main", "develop"}, updated.WatchedBranches)
	require.Equal(t, other.UserID, *updated.EnabledBy, "whoever re-confirms becomes accountable")
}

func TestGet_NilWhenOff(t *testing.T) {
	h := newHarness(t, 10)
	w, err := h.svc.Get(context.Background(), h.actor, h.projects.projectID)
	require.NoError(t, err)
	require.Nil(t, w)
}

func TestDisable_RemovesHookAndRow(t *testing.T) {
	h := newHarness(t, 10)
	h.enable(t)
	res, err := h.svc.Disable(context.Background(), h.actor, h.projects.projectID)
	require.NoError(t, err)
	require.True(t, res.GitHubHookRemoved)
	require.Equal(t, []int64{777}, h.hooks.deleted)
	require.Empty(t, h.repo.rows)
}

func TestDisable_StillTurnsOffWhenGitHubRefuses(t *testing.T) {
	h := newHarness(t, 10)
	h.enable(t)
	h.hooks.deleteErr = errors.New("token revoked")
	res, err := h.svc.Disable(context.Background(), h.actor, h.projects.projectID)
	require.NoError(t, err)
	require.False(t, res.GitHubHookRemoved)
	require.Empty(t, h.repo.rows)
}

// --- Receive ---

func TestReceive_BadSignatureIsRejectedBeforeAnythingIsQueued(t *testing.T) {
	h := newHarness(t, 10)
	w := h.enable(t)
	body := pushBody("acme/payments-api", "main", "abc", "octocat")

	for name, sig := range map[string]string{
		"missing":      "",
		"wrong secret": github.Sign([]byte("not-the-secret"), body),
		"other body":   github.Sign(h.secret(t), []byte(`{}`)),
	} {
		err := h.svc.Receive(context.Background(), livescan.Delivery{WebhookID: w.ID, Event: "push", DeliveryID: "d-" + name, Body: body, Signature: sig})
		requireCode(t, err, "webhook.signature_invalid")
	}
	require.Empty(t, h.coord.events)
	require.Empty(t, h.coord.seen, "a forged request must not even reach dedup")
	require.Contains(t, h.audit.actions(), "webhook.signature_invalid")
}

func TestReceive_UnknownWebhookIs404(t *testing.T) {
	h := newHarness(t, 10)
	err := h.svc.Receive(context.Background(), livescan.Delivery{WebhookID: uuid.New(), Event: "push", Body: []byte(`{}`)})
	requireCode(t, err, "webhook.not_found")
}

func TestReceive_SameDeliveryTwiceIsQueuedOnce(t *testing.T) {
	h := newHarness(t, 10)
	h.enable(t)
	body := pushBody("acme/payments-api", "main", "abc", "octocat")
	require.NoError(t, h.deliver(t, "push", "delivery-1", body))
	require.NoError(t, h.deliver(t, "push", "delivery-1", body))
	require.Len(t, h.coord.events, 1)
}

func TestReceive_PingAndOtherEventsAreNotQueued(t *testing.T) {
	h := newHarness(t, 10)
	h.enable(t)
	require.NoError(t, h.deliver(t, "ping", "p1", []byte(`{"zen":"Keep it logically awesome."}`)))
	require.NoError(t, h.deliver(t, "issues", "i1", []byte(`{}`)))
	require.Empty(t, h.coord.events)
	require.Equal(t, "ignored_event", h.repo.only().LastDeliveryStatus)
}

// --- worker: push ---

func TestPush_RapidPushesCollapseIntoOneScan(t *testing.T) {
	h := newHarness(t, 10)
	h.enable(t)
	for i, sha := range []string{"sha1", "sha2", "sha3"} {
		require.NoError(t, h.deliver(t, "push", fmt.Sprintf("d%d", i), pushBody("acme/payments-api", "main", sha, "octocat")))
	}
	h.drain(t)

	require.Len(t, h.scans.calls, 1)
	call := h.scans.calls[0]
	require.Equal(t, domain.TriggerWebhookPush, call.in.TriggerSource)
	require.Equal(t, "main", call.in.Branch)
	require.Equal(t, "main", call.in.TriggerRef)
	require.Equal(t, "octocat", call.in.TriggerActor)
	require.Equal(t, domain.ScanTypePartial, call.in.Type)
	require.Equal(t, "sha3", h.audit.last("webhook.scan_triggered").Detail["commit_sha"], "the scan is for the newest push")
}

func TestPush_ScanIsAttributedToWhoeverEnabledIt(t *testing.T) {
	h := newHarness(t, 10)
	h.enable(t)
	require.NoError(t, h.deliver(t, "push", "d1", pushBody("acme/payments-api", "main", "abc", "someone-else")))
	h.drain(t)

	require.Len(t, h.scans.calls, 1)
	require.Equal(t, h.actor.UserID, h.scans.calls[0].actor.UserID)
	require.Equal(t, h.actor.OrgID, h.scans.calls[0].actor.OrgID)
}

func TestPush_IgnoredCases(t *testing.T) {
	cases := map[string][]byte{
		"unwatched branch": pushBody("acme/payments-api", "feature/x", "abc", "octocat"),
		"tag push":         []byte(`{"ref":"refs/tags/v1","after":"abc","repository":{"full_name":"acme/payments-api"}}`),
		"branch deleted":   []byte(`{"ref":"refs/heads/main","deleted":true,"repository":{"full_name":"acme/payments-api"}}`),
		"different repo":   pushBody("evil/other-repo", "main", "abc", "octocat"),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, 10)
			h.enable(t)
			require.NoError(t, h.deliver(t, "push", "d1", body))
			h.drain(t)
			require.Empty(t, h.scans.calls)
		})
	}
}

func TestPush_RepositoryMatchIsCaseInsensitive(t *testing.T) {
	h := newHarness(t, 10)
	h.enable(t)
	require.NoError(t, h.deliver(t, "push", "d1", pushBody("ACME/Payments-API", "main", "abc", "octocat")))
	h.drain(t)
	require.Len(t, h.scans.calls, 1)
}

func TestPush_StoredPentestNeverReachesCreateScan(t *testing.T) {
	h := newHarness(t, 10)
	w := h.enable(t)
	// Simulate a row that somehow holds pentest (the DB CHECK should make
	// this impossible — this proves the worker doesn't rely on that alone).
	row := h.repo.rows[w.ID]
	row.Engines = []domain.EngineID{domain.EnginePentest, domain.EngineCodeScan}
	h.repo.rows[w.ID] = row

	require.NoError(t, h.deliver(t, "push", "d1", pushBody("acme/payments-api", "main", "abc", "octocat")))
	h.drain(t)
	require.Len(t, h.scans.calls, 1)
	require.Equal(t, []domain.EngineID{domain.EngineCodeScan}, h.scans.calls[0].in.Engines)
}

func TestPush_TurnedOffWhileWaitingMeansNoScan(t *testing.T) {
	h := newHarness(t, 10)
	h.enable(t)
	require.NoError(t, h.deliver(t, "push", "d1", pushBody("acme/payments-api", "main", "abc", "octocat")))
	raw, _ := h.coord.PopEvent(context.Background(), 0)
	require.NoError(t, h.svc.HandleQueuedEvent(context.Background(), raw))

	_, err := h.svc.Disable(context.Background(), h.actor, h.projects.projectID)
	require.NoError(t, err)
	require.NoError(t, h.svc.FireDue(context.Background(), time.Now().Add(time.Hour)))
	require.Empty(t, h.scans.calls)
}

// --- worker: pull requests ---

func TestPullRequest_ScansHeadImmediately(t *testing.T) {
	h := newHarness(t, 10)
	h.enable(t)
	require.NoError(t, h.deliver(t, "pull_request", "d1", prBody("opened", 12, "acme/payments-api", "acme/payments-api", "fix-login", "main")))
	raw, _ := h.coord.PopEvent(context.Background(), 0)
	require.NoError(t, h.svc.HandleQueuedEvent(context.Background(), raw))

	require.Len(t, h.scans.calls, 1, "no debounce for pull requests")
	in := h.scans.calls[0].in
	require.Equal(t, domain.TriggerWebhookPullRequest, in.TriggerSource)
	require.Equal(t, "fix-login", in.Branch)
	require.Equal(t, "refs/pull/12/head", in.TriggerRef)
}

func TestPullRequest_IgnoredCases(t *testing.T) {
	cases := map[string][]byte{
		"closed":         prBody("closed", 1, "acme/payments-api", "acme/payments-api", "x", "main"),
		"labeled":        prBody("labeled", 1, "acme/payments-api", "acme/payments-api", "x", "main"),
		"from a fork":    prBody("opened", 1, "stranger/payments-api", "acme/payments-api", "main", "main"),
		"into unwatched": prBody("opened", 1, "acme/payments-api", "acme/payments-api", "x", "release"),
		"different repo": prBody("opened", 1, "evil/r", "evil/r", "x", "main"),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, 10)
			h.enable(t)
			require.NoError(t, h.deliver(t, "pull_request", "d1", body))
			h.drain(t)
			require.Empty(t, h.scans.calls)
		})
	}
}

// --- limits ---

func TestRateLimit_DropsOverCapAndCircuitBreakerPauses(t *testing.T) {
	h := newHarness(t, 2)
	h.enable(t)
	for i := 1; i <= 6; i++ {
		require.NoError(t, h.deliver(t, "pull_request", fmt.Sprintf("d%d", i),
			prBodyAt(fmt.Sprintf("sha%d", i), "synchronize", 5, "acme/payments-api", "acme/payments-api", "feat", "main")))
	}
	h.drain(t)

	require.Len(t, h.scans.calls, 2, "only the hourly cap's worth of scans start")
	require.Contains(t, h.audit.actions(), "webhook.rate_limited")
	require.Contains(t, h.audit.actions(), "webhook.paused")
	w := h.repo.only()
	require.NotNil(t, w.PausedReason)
	require.True(t, strings.Contains(*w.PausedReason, "paused automatically"))

	// Paused means nothing more happens, even after the counter would reset.
	h.coord.counters = map[string]int64{}
	require.NoError(t, h.deliver(t, "pull_request", "d7", prBodyAt("sha7", "synchronize", 5, "acme/payments-api", "acme/payments-api", "feat", "main")))
	h.drain(t)
	require.Len(t, h.scans.calls, 2)
}

func TestCreateScanFailureIsRecordedNotRetriedForever(t *testing.T) {
	h := newHarness(t, 10)
	h.enable(t)
	h.scans.err = apperrors.Unprocessable("scan.engine_unavailable", "codescan is not registered")
	require.NoError(t, h.deliver(t, "pull_request", "d1", prBody("opened", 1, "acme/payments-api", "acme/payments-api", "x", "main")))
	h.drain(t)

	entry := h.audit.last("webhook.delivery_failed")
	require.NotNil(t, entry)
	require.Equal(t, "scan.engine_unavailable", entry.Detail["reason"])
	require.Equal(t, "scan_failed", h.repo.only().LastDeliveryStatus)
}

func TestQueuedEventPayloadRoundTrips(t *testing.T) {
	h := newHarness(t, 10)
	w := h.enable(t)
	require.NoError(t, h.deliver(t, "push", "d1", pushBody("acme/payments-api", "main", "abc", "octocat")))
	var ev map[string]any
	require.NoError(t, json.Unmarshal(h.coord.events[0], &ev))
	require.Equal(t, w.ID.String(), ev["webhook_id"])
	require.Equal(t, "push", ev["event"])
}
