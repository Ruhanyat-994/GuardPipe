// Package notification tells people when a scan finishes: an entry in the
// in-app feed (the bell) and, if they want it, an email with the PDF report
// attached.
//
// Nothing here runs inside a scan. The orchestrator's worker calls
// ScanFinished once a scan's last job is done; that only writes rows (a
// notification, and an outbox entry for the email). Sender, a separate
// loop in the worker process, then builds the PDF and hands it to the
// Mailer, retrying with backoff — a slow or unavailable mail provider can
// never hold up or fail a scan.
//
// Who gets told: the scan's triggered_by user — the person who started a
// manual scan, who created the schedule, or who turned live scanning on
// (the same person the report's "Authorisation & responsibility" section
// names). Where the email goes: that user's verified report address, or
// their account email if they haven't set one. A new report address is
// only used once someone clicks the verification link sent to it.
package notification

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// Notification kinds (notifications.kind).
const (
	KindScanCompleted = "scan_completed"
	KindScanFailed    = "scan_failed"
	KindScanCancelled = "scan_cancelled"
)

// Settings mirrors one user_notification_settings row, or the defaults for a
// user who has never changed anything.
type Settings struct {
	UserID uuid.UUID
	// ReportEmail is the verified address reports go to; nil = account email.
	ReportEmail *string
	// PendingEmail is an address waiting for its verification link to be
	// clicked. Never used for sending reports.
	PendingEmail        *string
	PendingExpiresAt    *time.Time
	EmailOnScanComplete bool
	EmailOnLiveScan     bool
}

// DefaultSettings is what a user with no settings row gets.
func DefaultSettings(userID uuid.UUID) Settings {
	return Settings{UserID: userID, EmailOnScanComplete: true, EmailOnLiveScan: false}
}

// SettingsView is Settings as the settings page shows it.
type SettingsView struct {
	AccountEmail string
	// ReportEmail is where reports go right now — the verified override, or
	// the account email.
	ReportEmail         string
	UsingAccountEmail   bool
	PendingEmail        *string
	PendingExpiresAt    *time.Time
	EmailOnScanComplete bool
	EmailOnLiveScan     bool
	// EmailEnabled is false when the server has no mail backend configured
	// at all (GUARDPIPE_MAIL_BACKEND=off) — the page says so rather than
	// offering settings that do nothing.
	EmailEnabled bool
}

// UpdateSettingsInput — every field optional; nil leaves it unchanged.
type UpdateSettingsInput struct {
	EmailOnScanComplete *bool
	EmailOnLiveScan     *bool
	// ReportEmail: a new address starts verification; "" or the account
	// email itself switches back to the account email immediately.
	ReportEmail *string
}

// ScanInfo is what a notification or report email needs to know about a
// finished scan.
type ScanInfo struct {
	ScanID        uuid.UUID
	ProjectID     uuid.UUID
	OrgID         uuid.UUID
	ProjectName   string
	ScanNumber    int
	Type          domain.ScanType
	Status        domain.ScanStatus
	TriggeredBy   *uuid.UUID
	TriggerSource domain.TriggerSource
	TriggerRef    *string
	Branch        *string
	FindingCounts map[domain.Severity]int
	// Score/Verdict are nil when the scan has no risk assessment (failed,
	// cancelled, or scoring didn't run).
	Score   *int
	Verdict *string
}

// IsLiveScan reports whether a GitHub push or pull request started the scan.
func (s ScanInfo) IsLiveScan() bool {
	return s.TriggerSource == domain.TriggerWebhookPush || s.TriggerSource == domain.TriggerWebhookPullRequest
}

// OutboxEmail is one scan_report_emails row being delivered.
type OutboxEmail struct {
	ID        uuid.UUID
	ScanID    uuid.UUID
	UserID    uuid.UUID
	OrgID     uuid.UUID
	Recipient string
	// Attempts counts this delivery attempt too (the claim increments it).
	Attempts int
}

// Notification is one row of the in-app feed.
type Notification struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	OrgID     uuid.UUID
	Kind      string
	ScanID    *uuid.UUID
	Title     string
	Body      string
	ReadAt    *time.Time
	CreatedAt time.Time
}

// Email is one outgoing message. HTML and Text carry the same content; the
// Mailer sends both as multipart/alternative.
type Email struct {
	To          string
	Subject     string
	Text        string
	HTML        string
	Attachments []Attachment
}

// Attachment is one file attached to an Email.
type Attachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// Mailer delivers one email. Implemented by internal/adapters/mailer (log,
// SMTP, Amazon SES). An error means "not delivered, try again later".
type Mailer interface {
	Send(ctx context.Context, e Email) error
}

// ReportRenderer builds a scan's PDF report as the scan's own org would
// see it — main.go adapts reporting.Assembler + reporting.RenderPDF.
type ReportRenderer interface {
	RenderScanPDF(ctx context.Context, orgID, scanID uuid.UUID) ([]byte, error)
}

// Repository is implemented by internal/store/repo.NotificationRepo.
type Repository interface {
	GetSettings(ctx context.Context, userID uuid.UUID) (Settings, error)
	SaveToggles(ctx context.Context, userID uuid.UUID, onScanComplete, onLiveScan bool) error
	SetPendingEmail(ctx context.Context, userID uuid.UUID, email string, tokenHash []byte, expiresAt time.Time) error
	// UseAccountEmail clears both the verified override and any pending one.
	UseAccountEmail(ctx context.Context, userID uuid.UUID) error
	// ConfirmPendingEmail promotes the pending address matching tokenHash
	// (and not yet expired at now) to the verified report address. Returns
	// a NotFound error when there's no such pending address.
	ConfirmPendingEmail(ctx context.Context, tokenHash []byte, now time.Time) (userID uuid.UUID, email string, err error)

	GetUserContact(ctx context.Context, userID uuid.UUID) (email, displayName string, err error)
	GetScanInfo(ctx context.Context, scanID uuid.UUID) (*ScanInfo, error)

	// EnqueueReportEmail is a no-op if this scan was already queued for
	// this user.
	EnqueueReportEmail(ctx context.Context, e OutboxEmail) error
	// ClaimDueEmails leases up to limit due emails for lease, incrementing
	// their attempt count. A lease that runs out (the sender crashed) makes
	// the email due again.
	ClaimDueEmails(ctx context.Context, limit int, lease time.Duration) ([]OutboxEmail, error)
	MarkEmailSent(ctx context.Context, id uuid.UUID) error
	MarkEmailRetry(ctx context.Context, id uuid.UUID, nextAttempt time.Time, lastError string) error
	MarkEmailFailed(ctx context.Context, id uuid.UUID, lastError string) error

	// CreateNotification is a no-op if this scan already notified this user.
	CreateNotification(ctx context.Context, n Notification) error
	ListNotifications(ctx context.Context, userID, orgID uuid.UUID, limit int) ([]Notification, int, error)
	MarkNotificationRead(ctx context.Context, userID, orgID, id uuid.UUID) error
	MarkAllNotificationsRead(ctx context.Context, userID, orgID uuid.UUID) error
}
