package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/notification"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// NotificationRepo implements notification.Repository against
// user_notification_settings, scan_report_emails and notifications
// (migration 00029), plus read-only lookups on users/scans/projects.
type NotificationRepo struct {
	db Querier
}

func NewNotificationRepo(db Querier) *NotificationRepo {
	return &NotificationRepo{db: db}
}

var _ notification.Repository = (*NotificationRepo)(nil)

func (r *NotificationRepo) GetSettings(ctx context.Context, userID uuid.UUID) (notification.Settings, error) {
	const q = `
		SELECT report_email::text, pending_email::text, pending_expires_at, email_on_scan_complete, email_on_live_scan
		FROM user_notification_settings WHERE user_id = $1`
	s := notification.DefaultSettings(userID)
	err := r.db.QueryRow(ctx, q, userID).Scan(&s.ReportEmail, &s.PendingEmail, &s.PendingExpiresAt, &s.EmailOnScanComplete, &s.EmailOnLiveScan)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("repo: get notification settings: %w", err)
	}
	return s, nil
}

func (r *NotificationRepo) SaveToggles(ctx context.Context, userID uuid.UUID, onScanComplete, onLiveScan bool) error {
	const q = `
		INSERT INTO user_notification_settings (user_id, email_on_scan_complete, email_on_live_scan)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE
		SET email_on_scan_complete = EXCLUDED.email_on_scan_complete,
			email_on_live_scan = EXCLUDED.email_on_live_scan,
			updated_at = now()`
	if _, err := r.db.Exec(ctx, q, userID, onScanComplete, onLiveScan); err != nil {
		return fmt.Errorf("repo: save notification toggles: %w", err)
	}
	return nil
}

func (r *NotificationRepo) SetPendingEmail(ctx context.Context, userID uuid.UUID, email string, tokenHash []byte, expiresAt time.Time) error {
	const q = `
		INSERT INTO user_notification_settings (user_id, pending_email, pending_token_hash, pending_expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO UPDATE
		SET pending_email = EXCLUDED.pending_email,
			pending_token_hash = EXCLUDED.pending_token_hash,
			pending_expires_at = EXCLUDED.pending_expires_at,
			updated_at = now()`
	if _, err := r.db.Exec(ctx, q, userID, email, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("repo: set pending report email: %w", err)
	}
	return nil
}

func (r *NotificationRepo) UseAccountEmail(ctx context.Context, userID uuid.UUID) error {
	const q = `
		UPDATE user_notification_settings
		SET report_email = NULL, pending_email = NULL, pending_token_hash = NULL, pending_expires_at = NULL, updated_at = now()
		WHERE user_id = $1`
	if _, err := r.db.Exec(ctx, q, userID); err != nil {
		return fmt.Errorf("repo: reset report email: %w", err)
	}
	return nil
}

func (r *NotificationRepo) ConfirmPendingEmail(ctx context.Context, tokenHash []byte, now time.Time) (uuid.UUID, string, error) {
	const q = `
		UPDATE user_notification_settings
		SET report_email = pending_email,
			pending_email = NULL, pending_token_hash = NULL, pending_expires_at = NULL,
			updated_at = now()
		WHERE pending_token_hash = $1 AND pending_expires_at > $2
		RETURNING user_id, report_email::text`
	var userID uuid.UUID
	var email string
	err := r.db.QueryRow(ctx, q, tokenHash, now).Scan(&userID, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", apperrors.NotFound("notification.invalid_token", "verification token not found")
	}
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("repo: confirm report email: %w", err)
	}
	return userID, email, nil
}

func (r *NotificationRepo) GetUserContact(ctx context.Context, userID uuid.UUID) (string, string, error) {
	var email, name string
	err := r.db.QueryRow(ctx, `SELECT email::text, display_name FROM users WHERE id = $1`, userID).Scan(&email, &name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", apperrors.NotFound("user.not_found", "user not found")
	}
	if err != nil {
		return "", "", fmt.Errorf("repo: get user contact: %w", err)
	}
	return email, name, nil
}

func (r *NotificationRepo) GetScanInfo(ctx context.Context, scanID uuid.UUID) (*notification.ScanInfo, error) {
	const q = `
		SELECT s.id, s.project_id, p.org_id, p.name,
			(SELECT count(*) FROM scans s2 WHERE s2.project_id = s.project_id AND s2.created_at <= s.created_at),
			s.type, s.status, s.triggered_by, s.trigger_source, s.trigger_ref, s.branch, s.finding_counts,
			ra.score, ra.verdict::text
		FROM scans s
		JOIN projects p ON p.id = s.project_id
		LEFT JOIN risk_assessments ra ON ra.scan_id = s.id
		WHERE s.id = $1`
	var info notification.ScanInfo
	var scanType, status string
	var triggerSource *string
	var findingCounts map[string]int
	err := r.db.QueryRow(ctx, q, scanID).Scan(
		&info.ScanID, &info.ProjectID, &info.OrgID, &info.ProjectName, &info.ScanNumber,
		&scanType, &status, &info.TriggeredBy, &triggerSource, &info.TriggerRef, &info.Branch, &findingCounts,
		&info.Score, &info.Verdict,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.NotFound("scan.not_found", "scan not found")
	}
	if err != nil {
		return nil, fmt.Errorf("repo: get scan info: %w", err)
	}
	info.Type = domain.ScanType(scanType)
	info.Status = domain.ScanStatus(status)
	if triggerSource != nil {
		info.TriggerSource = domain.TriggerSource(*triggerSource)
	}
	info.FindingCounts = stringMapToSeverityMap(findingCounts)
	return &info, nil
}

func (r *NotificationRepo) EnqueueReportEmail(ctx context.Context, e notification.OutboxEmail) error {
	const q = `
		INSERT INTO scan_report_emails (scan_id, user_id, org_id, recipient)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (scan_id, user_id) DO NOTHING`
	if _, err := r.db.Exec(ctx, q, e.ScanID, e.UserID, e.OrgID, e.Recipient); err != nil {
		return fmt.Errorf("repo: enqueue report email: %w", err)
	}
	return nil
}

// ClaimDueEmails leases due rows with FOR UPDATE SKIP LOCKED, so two worker
// replicas never pick up the same email.
func (r *NotificationRepo) ClaimDueEmails(ctx context.Context, limit int, lease time.Duration) ([]notification.OutboxEmail, error) {
	const q = `
		UPDATE scan_report_emails e
		SET status = 'sending', attempts = e.attempts + 1,
			next_attempt_at = now() + make_interval(secs => $2), updated_at = now()
		WHERE e.id IN (
			SELECT id FROM scan_report_emails
			WHERE status IN ('pending', 'sending') AND next_attempt_at <= now()
			ORDER BY next_attempt_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING e.id, e.scan_id, e.user_id, e.org_id, e.recipient::text, e.attempts`
	rows, err := r.db.Query(ctx, q, limit, lease.Seconds())
	if err != nil {
		return nil, fmt.Errorf("repo: claim report emails: %w", err)
	}
	defer rows.Close()
	var out []notification.OutboxEmail
	for rows.Next() {
		var e notification.OutboxEmail
		if err := rows.Scan(&e.ID, &e.ScanID, &e.UserID, &e.OrgID, &e.Recipient, &e.Attempts); err != nil {
			return nil, fmt.Errorf("repo: scan claimed report email: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: iterate claimed report emails: %w", err)
	}
	return out, nil
}

func (r *NotificationRepo) MarkEmailSent(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE scan_report_emails SET status = 'sent', sent_at = now(), last_error = NULL, updated_at = now() WHERE id = $1`
	if _, err := r.db.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("repo: mark report email sent: %w", err)
	}
	return nil
}

func (r *NotificationRepo) MarkEmailRetry(ctx context.Context, id uuid.UUID, nextAttempt time.Time, lastError string) error {
	const q = `UPDATE scan_report_emails SET status = 'pending', next_attempt_at = $2, last_error = $3, updated_at = now() WHERE id = $1`
	if _, err := r.db.Exec(ctx, q, id, nextAttempt, lastError); err != nil {
		return fmt.Errorf("repo: schedule report email retry: %w", err)
	}
	return nil
}

func (r *NotificationRepo) MarkEmailFailed(ctx context.Context, id uuid.UUID, lastError string) error {
	const q = `UPDATE scan_report_emails SET status = 'failed', last_error = $2, updated_at = now() WHERE id = $1`
	if _, err := r.db.Exec(ctx, q, id, lastError); err != nil {
		return fmt.Errorf("repo: mark report email failed: %w", err)
	}
	return nil
}

func (r *NotificationRepo) CreateNotification(ctx context.Context, n notification.Notification) error {
	const q = `
		INSERT INTO notifications (user_id, org_id, kind, scan_id, title, body)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (scan_id, user_id) DO NOTHING`
	if _, err := r.db.Exec(ctx, q, n.UserID, n.OrgID, n.Kind, n.ScanID, n.Title, n.Body); err != nil {
		return fmt.Errorf("repo: create notification: %w", err)
	}
	return nil
}

func (r *NotificationRepo) ListNotifications(ctx context.Context, userID, orgID uuid.UUID, limit int) ([]notification.Notification, int, error) {
	var unread int
	if err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM notifications WHERE user_id = $1 AND org_id = $2 AND read_at IS NULL`,
		userID, orgID,
	).Scan(&unread); err != nil {
		return nil, 0, fmt.Errorf("repo: count unread notifications: %w", err)
	}

	const q = `
		SELECT id, user_id, org_id, kind, scan_id, title, body, read_at, created_at
		FROM notifications
		WHERE user_id = $1 AND org_id = $2
		ORDER BY created_at DESC
		LIMIT $3`
	rows, err := r.db.Query(ctx, q, userID, orgID, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("repo: list notifications: %w", err)
	}
	defer rows.Close()
	var out []notification.Notification
	for rows.Next() {
		var n notification.Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.OrgID, &n.Kind, &n.ScanID, &n.Title, &n.Body, &n.ReadAt, &n.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("repo: scan notification: %w", err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("repo: iterate notifications: %w", err)
	}
	return out, unread, nil
}

func (r *NotificationRepo) MarkNotificationRead(ctx context.Context, userID, orgID, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE notifications SET read_at = COALESCE(read_at, now()) WHERE id = $1 AND user_id = $2 AND org_id = $3`,
		id, userID, orgID,
	)
	if err != nil {
		return fmt.Errorf("repo: mark notification read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("notification.not_found", "notification not found")
	}
	return nil
}

func (r *NotificationRepo) MarkAllNotificationsRead(ctx context.Context, userID, orgID uuid.UUID) error {
	_, err := r.db.Exec(ctx,
		`UPDATE notifications SET read_at = now() WHERE user_id = $1 AND org_id = $2 AND read_at IS NULL`,
		userID, orgID,
	)
	if err != nil {
		return fmt.Errorf("repo: mark all notifications read: %w", err)
	}
	return nil
}
