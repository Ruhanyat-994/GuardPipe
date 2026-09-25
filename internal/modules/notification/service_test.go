package notification_test

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/notification"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

type harness struct {
	svc    *notification.Service
	repo   *fakeRepo
	mailer *fakeMailer
	audit  *fakeAudit
	user   uuid.UUID
	org    uuid.UUID
	actor  domain.Actor
}

func newHarness(t *testing.T, emailEnabled bool) *harness {
	t.Helper()
	h := &harness{repo: newFakeRepo(), mailer: &fakeMailer{}, audit: &fakeAudit{}, user: uuid.New(), org: uuid.New()}
	h.repo.users[h.user] = [2]string{"owner@example.com", "Owner"}
	h.actor = domain.Actor{UserID: h.user, OrgID: h.org, Role: domain.RoleAdmin}
	h.svc = notification.NewService(notification.Config{AppURL: "https://app.example.com/", EmailEnabled: emailEnabled}, h.repo, h.mailer, h.audit, nil)
	return h
}

func (h *harness) addScan(status domain.ScanStatus, source domain.TriggerSource) uuid.UUID {
	id := uuid.New()
	score, verdict := 72, "warn"
	h.repo.scans[id] = &notification.ScanInfo{
		ScanID: id, ProjectID: uuid.New(), OrgID: h.org, ProjectName: "Payments API", ScanNumber: 4,
		Status: status, TriggeredBy: &h.user, TriggerSource: source,
		FindingCounts: map[domain.Severity]int{domain.SeverityHigh: 2},
		Score:         &score, Verdict: &verdict,
	}
	return id
}

func TestScanFinished_CompletedManualScan_FeedEntryAndEmailToAccountAddress(t *testing.T) {
	h := newHarness(t, true)
	scanID := h.addScan(domain.ScanStatusCompleted, domain.TriggerManual)

	require.NoError(t, h.svc.ScanFinished(context.Background(), scanID))

	require.Len(t, h.repo.feed, 1)
	require.Equal(t, notification.KindScanCompleted, h.repo.feed[0].Kind)
	require.Contains(t, h.repo.feed[0].Body, "risk score 72 (warn)")
	require.Len(t, h.repo.outbox, 1)
	require.Equal(t, "owner@example.com", h.repo.outbox[0].Recipient)
	require.Empty(t, h.mailer.sent, "ScanFinished only queues — it never sends itself")
}

func TestScanFinished_IsIdempotent(t *testing.T) {
	h := newHarness(t, true)
	scanID := h.addScan(domain.ScanStatusCompleted, domain.TriggerManual)
	require.NoError(t, h.svc.ScanFinished(context.Background(), scanID))
	require.NoError(t, h.svc.ScanFinished(context.Background(), scanID))
	require.Len(t, h.repo.feed, 1)
	require.Len(t, h.repo.outbox, 1)
}

func TestScanFinished_UsesVerifiedReportAddress(t *testing.T) {
	h := newHarness(t, true)
	verified := "security@example.org"
	s := notification.DefaultSettings(h.user)
	s.ReportEmail = &verified
	h.repo.settings[h.user] = s

	require.NoError(t, h.svc.ScanFinished(context.Background(), h.addScan(domain.ScanStatusCompleted, domain.TriggerScheduled)))
	require.Equal(t, verified, h.repo.outbox[0].Recipient)
}

// Near-misses: every case where a report must NOT be emailed.
func TestScanFinished_NoEmail(t *testing.T) {
	cases := []struct {
		name     string
		status   domain.ScanStatus
		source   domain.TriggerSource
		settings func(*notification.Settings)
		enabled  bool
		wantFeed bool
	}{
		{name: "cancelled scan", status: domain.ScanStatusCancelled, source: domain.TriggerManual, enabled: true, wantFeed: true},
		{name: "email turned off", status: domain.ScanStatusCompleted, source: domain.TriggerManual, enabled: true, wantFeed: true,
			settings: func(s *notification.Settings) { s.EmailOnScanComplete = false }},
		{name: "live scan with live emails off (the default)", status: domain.ScanStatusCompleted, source: domain.TriggerWebhookPush, enabled: true, wantFeed: true},
		{name: "mail backend off", status: domain.ScanStatusCompleted, source: domain.TriggerManual, enabled: false, wantFeed: true},
		{name: "scan still running", status: domain.ScanStatusRunning, source: domain.TriggerManual, enabled: true, wantFeed: false},
		{name: "pending address is never used", status: domain.ScanStatusCompleted, source: domain.TriggerManual, enabled: true, wantFeed: true,
			settings: func(s *notification.Settings) { p := "attacker@evil.test"; s.PendingEmail = &p }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, tc.enabled)
			if tc.settings != nil {
				s := notification.DefaultSettings(h.user)
				tc.settings(&s)
				h.repo.settings[h.user] = s
			}
			require.NoError(t, h.svc.ScanFinished(context.Background(), h.addScan(tc.status, tc.source)))
			if tc.wantFeed {
				require.Len(t, h.repo.feed, 1)
			} else {
				require.Empty(t, h.repo.feed)
			}
			for _, e := range h.repo.outbox {
				require.NotEqual(t, "attacker@evil.test", e.Recipient, "an unverified address must never receive a report")
			}
			if tc.name != "pending address is never used" {
				require.Empty(t, h.repo.outbox)
			}
		})
	}
}

func TestScanFinished_LiveScanWithLiveEmailsOn_Queues(t *testing.T) {
	h := newHarness(t, true)
	s := notification.DefaultSettings(h.user)
	s.EmailOnLiveScan = true
	h.repo.settings[h.user] = s
	require.NoError(t, h.svc.ScanFinished(context.Background(), h.addScan(domain.ScanStatusCompleted, domain.TriggerWebhookPush)))
	require.Len(t, h.repo.outbox, 1)
}

func TestScanFinished_NoTriggeringUser_DoesNothing(t *testing.T) {
	h := newHarness(t, true)
	id := h.addScan(domain.ScanStatusCompleted, domain.TriggerManual)
	h.repo.scans[id].TriggeredBy = nil
	require.NoError(t, h.svc.ScanFinished(context.Background(), id))
	require.Empty(t, h.repo.feed)
	require.Empty(t, h.repo.outbox)
}

func tokenFrom(t *testing.T, e notification.Email) string {
	t.Helper()
	i := strings.Index(e.Text, "https://app.example.com/verify-report-email?token=")
	require.GreaterOrEqual(t, i, 0, "verification link missing: %s", e.Text)
	link := strings.Fields(e.Text[i:])[0]
	u, err := url.Parse(link)
	require.NoError(t, err)
	return u.Query().Get("token")
}

func TestChangeReportEmail_RequiresVerificationBeforeUse(t *testing.T) {
	h := newHarness(t, true)
	ctx := context.Background()
	newAddr := "security@example.org"

	view, err := h.svc.UpdateSettings(ctx, h.actor, notification.UpdateSettingsInput{ReportEmail: &newAddr})
	require.NoError(t, err)
	require.Equal(t, "owner@example.com", view.ReportEmail, "still the account email until verified")
	require.NotNil(t, view.PendingEmail)
	require.Len(t, h.mailer.sent, 1)
	require.Equal(t, newAddr, h.mailer.sent[0].To)

	// A report finishing now still goes to the old address.
	require.NoError(t, h.svc.ScanFinished(ctx, h.addScan(domain.ScanStatusCompleted, domain.TriggerManual)))
	require.Equal(t, "owner@example.com", h.repo.outbox[0].Recipient)

	confirmed, err := h.svc.ConfirmReportEmail(ctx, tokenFrom(t, h.mailer.sent[0]))
	require.NoError(t, err)
	require.Equal(t, newAddr, confirmed)

	view, err = h.svc.GetSettings(ctx, h.actor)
	require.NoError(t, err)
	require.Equal(t, newAddr, view.ReportEmail)
	require.False(t, view.UsingAccountEmail)
	require.Nil(t, view.PendingEmail)
	require.Contains(t, h.audit.actions, "notification.report_email_verified")
}

func TestConfirmReportEmail_TokenIsSingleUse(t *testing.T) {
	h := newHarness(t, true)
	ctx := context.Background()
	addr := "security@example.org"
	_, err := h.svc.UpdateSettings(ctx, h.actor, notification.UpdateSettingsInput{ReportEmail: &addr})
	require.NoError(t, err)
	token := tokenFrom(t, h.mailer.sent[0])

	_, err = h.svc.ConfirmReportEmail(ctx, token)
	require.NoError(t, err)
	_, err = h.svc.ConfirmReportEmail(ctx, token)
	requireKind(t, err, apperrors.KindNotFound)
}

func TestConfirmReportEmail_GarbageToken_NotFound(t *testing.T) {
	h := newHarness(t, true)
	for _, token := range []string{"", "not-base64!!", "c2hvcnQ"} {
		_, err := h.svc.ConfirmReportEmail(context.Background(), token)
		requireKind(t, err, apperrors.KindNotFound)
	}
}

func TestUpdateSettings_InvalidEmail_Rejected(t *testing.T) {
	h := newHarness(t, true)
	for _, bad := range []string{"not-an-email", "Name <a@b.co>", "a@b", "a@b.co\r\nBcc: x@y.z"} {
		_, err := h.svc.UpdateSettings(context.Background(), h.actor, notification.UpdateSettingsInput{ReportEmail: &bad})
		requireKind(t, err, apperrors.KindValidation)
	}
	require.Empty(t, h.mailer.sent)
}

func TestUpdateSettings_EmptyAddress_SwitchesBackToAccountEmail(t *testing.T) {
	h := newHarness(t, true)
	verified := "security@example.org"
	s := notification.DefaultSettings(h.user)
	s.ReportEmail = &verified
	h.repo.settings[h.user] = s

	empty := ""
	view, err := h.svc.UpdateSettings(context.Background(), h.actor, notification.UpdateSettingsInput{ReportEmail: &empty})
	require.NoError(t, err)
	require.True(t, view.UsingAccountEmail)
	require.Equal(t, "owner@example.com", view.ReportEmail)
	require.Empty(t, h.mailer.sent, "switching back needs no verification")
}

func TestUpdateSettings_Toggles(t *testing.T) {
	h := newHarness(t, true)
	off, on := false, true
	view, err := h.svc.UpdateSettings(context.Background(), h.actor, notification.UpdateSettingsInput{EmailOnScanComplete: &off, EmailOnLiveScan: &on})
	require.NoError(t, err)
	require.False(t, view.EmailOnScanComplete)
	require.True(t, view.EmailOnLiveScan)
}

func TestUpdateSettings_VerificationSendFails_ReportsError(t *testing.T) {
	h := newHarness(t, true)
	h.mailer.err = errors.New("smtp down")
	addr := "security@example.org"
	_, err := h.svc.UpdateSettings(context.Background(), h.actor, notification.UpdateSettingsInput{ReportEmail: &addr})
	requireKind(t, err, apperrors.KindExternal)
}

func TestSendTest_GoesToCurrentReportAddress(t *testing.T) {
	h := newHarness(t, true)
	to, err := h.svc.SendTest(context.Background(), h.actor)
	require.NoError(t, err)
	require.Equal(t, "owner@example.com", to)
	require.Len(t, h.mailer.sent, 1)
}

func TestFeed_ScopedToUserAndOrg(t *testing.T) {
	h := newHarness(t, true)
	require.NoError(t, h.svc.ScanFinished(context.Background(), h.addScan(domain.ScanStatusCompleted, domain.TriggerManual)))

	items, unread, err := h.svc.ListNotifications(context.Background(), h.actor)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, 1, unread)

	otherOrg := h.actor
	otherOrg.OrgID = uuid.New()
	items, _, err = h.svc.ListNotifications(context.Background(), otherOrg)
	require.NoError(t, err)
	require.Empty(t, items, "a notification from one org never shows in another")

	otherUser := domain.Actor{UserID: uuid.New(), OrgID: h.org}
	err = h.svc.MarkRead(context.Background(), otherUser, h.repo.feed[0].ID)
	requireKind(t, err, apperrors.KindNotFound)

	require.NoError(t, h.svc.MarkAllRead(context.Background(), h.actor))
	_, unread, err = h.svc.ListNotifications(context.Background(), h.actor)
	require.NoError(t, err)
	require.Zero(t, unread)
}

func requireKind(t *testing.T, err error, kind apperrors.Kind) {
	t.Helper()
	var appErr *apperrors.Error
	require.True(t, errors.As(err, &appErr), "want an app error, got %v", err)
	require.Equal(t, kind, appErr.Kind)
}
