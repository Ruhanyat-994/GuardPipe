//go:build integration

package repo_test

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/notification"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/store/repo"
)

func TestNotificationRepo_SettingsAndVerification(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	_, userID := seedOrgAndUser(t, pool)
	r := repo.NewNotificationRepo(pool)

	s, err := r.GetSettings(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, notification.DefaultSettings(userID), s, "no row yet = defaults")

	require.NoError(t, r.SaveToggles(ctx, userID, false, true))
	hash := sha256.Sum256([]byte("token"))
	require.NoError(t, r.SetPendingEmail(ctx, userID, "Security@Example.org", hash[:], time.Now().Add(time.Hour)))

	s, err = r.GetSettings(ctx, userID)
	require.NoError(t, err)
	require.False(t, s.EmailOnScanComplete)
	require.True(t, s.EmailOnLiveScan)
	require.Nil(t, s.ReportEmail)
	require.Equal(t, "Security@Example.org", *s.PendingEmail)

	// Expired: not confirmable.
	_, _, err = r.ConfirmPendingEmail(ctx, hash[:], time.Now().Add(2*time.Hour))
	require.Error(t, err)

	gotUser, email, err := r.ConfirmPendingEmail(ctx, hash[:], time.Now())
	require.NoError(t, err)
	require.Equal(t, userID, gotUser)
	require.Equal(t, "Security@Example.org", email)

	// Single use.
	_, _, err = r.ConfirmPendingEmail(ctx, hash[:], time.Now())
	require.Error(t, err)

	s, err = r.GetSettings(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, "Security@Example.org", *s.ReportEmail)
	require.Nil(t, s.PendingEmail)

	require.NoError(t, r.UseAccountEmail(ctx, userID))
	s, err = r.GetSettings(ctx, userID)
	require.NoError(t, err)
	require.Nil(t, s.ReportEmail)

	email, name, err := r.GetUserContact(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, "nadia@example.com", email)
	require.Equal(t, "Nadia R.", name)
}

func TestNotificationRepo_ScanInfoOutboxAndFeed(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	orgID, userID := seedOrgAndUser(t, pool)
	projectID := id.New()
	_, err := pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, status) VALUES ($1, $2, 'Payments API', 'active')`, projectID, orgID)
	require.NoError(t, err)

	scans := repo.NewScanRepo(pool)
	first := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}, TriggeredBy: &userID, TriggerSource: domain.TriggerManual}
	require.NoError(t, scans.Create(ctx, first))
	second := &domain.Scan{ID: id.New(), ProjectID: projectID, Type: domain.ScanTypeFullSupplyChain, Status: domain.ScanStatusQueued, RequestedEngines: []domain.EngineID{domain.EngineDepScan}, TriggeredBy: &userID, TriggerSource: domain.TriggerWebhookPush}
	require.NoError(t, scans.Create(ctx, second))
	_, err = pool.Exec(ctx, `UPDATE scans SET status = 'completed', finding_counts = '{"high": 2}' WHERE id = $1`, second.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO risk_assessments (scan_id, score, verdict, formula_version) VALUES ($1, 72, 'warn', '1.0')`, second.ID)
	require.NoError(t, err)

	r := repo.NewNotificationRepo(pool)
	info, err := r.GetScanInfo(ctx, second.ID)
	require.NoError(t, err)
	require.Equal(t, orgID, info.OrgID)
	require.Equal(t, "Payments API", info.ProjectName)
	require.Equal(t, 2, info.ScanNumber)
	require.Equal(t, domain.ScanStatusCompleted, info.Status)
	require.True(t, info.IsLiveScan())
	require.Equal(t, 2, info.FindingCounts[domain.SeverityHigh])
	require.Equal(t, 72, *info.Score)
	require.Equal(t, "warn", *info.Verdict)

	noScore, err := r.GetScanInfo(ctx, first.ID)
	require.NoError(t, err)
	require.Nil(t, noScore.Score)

	// Outbox: idempotent enqueue, claim leases, a claimed row isn't
	// claimable again until its lease runs out.
	out := notification.OutboxEmail{ScanID: second.ID, UserID: userID, OrgID: orgID, Recipient: "nadia@example.com"}
	require.NoError(t, r.EnqueueReportEmail(ctx, out))
	require.NoError(t, r.EnqueueReportEmail(ctx, out))
	claimed, err := r.ClaimDueEmails(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, 1, claimed[0].Attempts)
	again, err := r.ClaimDueEmails(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.Empty(t, again, "leased")

	require.NoError(t, r.MarkEmailRetry(ctx, claimed[0].ID, time.Now().Add(-time.Second), "throttled"))
	retried, err := r.ClaimDueEmails(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, retried, 1)
	require.Equal(t, 2, retried[0].Attempts)
	require.NoError(t, r.MarkEmailSent(ctx, retried[0].ID))
	done, err := r.ClaimDueEmails(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.Empty(t, done)

	// Feed: idempotent per scan+user, scoped to user and org.
	n := notification.Notification{UserID: userID, OrgID: orgID, Kind: notification.KindScanCompleted, ScanID: &second.ID, Title: "Scan finished", Body: "Payments API · Scan #2"}
	require.NoError(t, r.CreateNotification(ctx, n))
	require.NoError(t, r.CreateNotification(ctx, n))
	items, unread, err := r.ListNotifications(ctx, userID, orgID, 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, 1, unread)

	otherOrg, err := repo.NewOrganizationRepo(pool).Create(ctx, "Other Org")
	require.NoError(t, err)
	items, _, err = r.ListNotifications(ctx, userID, otherOrg, 10)
	require.NoError(t, err)
	require.Empty(t, items)
	require.Error(t, r.MarkNotificationRead(ctx, userID, otherOrg, n.ID), "another org's session can't touch it")

	all, _, err := r.ListNotifications(ctx, userID, orgID, 10)
	require.NoError(t, err)
	require.NoError(t, r.MarkNotificationRead(ctx, userID, orgID, all[0].ID))
	_, unread, err = r.ListNotifications(ctx, userID, orgID, 10)
	require.NoError(t, err)
	require.Zero(t, unread)
}
