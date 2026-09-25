package notification

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// verificationTTL is how long a report-address verification link works.
const verificationTTL = 24 * time.Hour

// feedLimit caps the bell's list — it's a recent-activity panel, not an
// archive.
const feedLimit = 30

// Config is the notification module's configuration.
type Config struct {
	// AppURL is the frontend's origin, used for every link in an email.
	AppURL string
	// EmailEnabled is false when no mail backend is configured: the feed
	// still works, nothing is queued for email.
	EmailEnabled bool
}

// Service is the notification module's API.
type Service struct {
	cfg    Config
	repo   Repository
	mailer Mailer
	audit  audit.Service
	log    *slog.Logger
	now    func() time.Time
}

// NewService wires the notification service. mailer may be nil only when
// cfg.EmailEnabled is false.
func NewService(cfg Config, repo Repository, mailer Mailer, auditSvc audit.Service, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	cfg.AppURL = strings.TrimRight(cfg.AppURL, "/")
	return &Service{cfg: cfg, repo: repo, mailer: mailer, audit: auditSvc, log: log, now: time.Now}
}

// ScanFinished records that a scan reached a terminal status: a feed entry
// for the user it's attributed to, and — if their settings say so — a
// queued report email. Called by the orchestrator's worker
// (orchestrator.ScanFinishedNotifier); must stay fast and never touch the
// mail provider itself.
func (s *Service) ScanFinished(ctx context.Context, scanID uuid.UUID) error {
	info, err := s.repo.GetScanInfo(ctx, scanID)
	if err != nil {
		return fmt.Errorf("notification: load scan: %w", err)
	}
	if info.TriggeredBy == nil {
		return nil // nobody to tell (the user was deleted, or a legacy scan)
	}
	userID := *info.TriggeredBy

	kind, title := feedKind(info.Status)
	if kind == "" {
		return nil // not a terminal status — nothing to announce yet
	}
	if err := s.repo.CreateNotification(ctx, Notification{
		UserID: userID, OrgID: info.OrgID, Kind: kind, ScanID: &info.ScanID,
		Title: title, Body: feedBody(info),
	}); err != nil {
		return fmt.Errorf("notification: create feed entry: %w", err)
	}

	// Cancelled scans produce no report worth emailing: the user cancelled
	// it themselves.
	if !s.cfg.EmailEnabled || info.Status == domain.ScanStatusCancelled {
		return nil
	}
	settings, err := s.repo.GetSettings(ctx, userID)
	if err != nil {
		return fmt.Errorf("notification: load settings: %w", err)
	}
	if !wantsEmail(settings, *info) {
		return nil
	}
	recipient, err := s.recipient(ctx, settings)
	if err != nil {
		return err
	}
	if err := s.repo.EnqueueReportEmail(ctx, OutboxEmail{
		ScanID: info.ScanID, UserID: userID, OrgID: info.OrgID, Recipient: recipient,
	}); err != nil {
		return fmt.Errorf("notification: queue report email: %w", err)
	}
	return nil
}

func wantsEmail(settings Settings, info ScanInfo) bool {
	if info.IsLiveScan() {
		return settings.EmailOnLiveScan
	}
	return settings.EmailOnScanComplete
}

func (s *Service) recipient(ctx context.Context, settings Settings) (string, error) {
	if settings.ReportEmail != nil {
		return *settings.ReportEmail, nil
	}
	email, _, err := s.repo.GetUserContact(ctx, settings.UserID)
	if err != nil {
		return "", fmt.Errorf("notification: load account email: %w", err)
	}
	return email, nil
}

func feedKind(status domain.ScanStatus) (kind, title string) {
	switch status {
	case domain.ScanStatusCompleted:
		return KindScanCompleted, "Scan finished"
	case domain.ScanStatusFailed:
		return KindScanFailed, "Scan failed"
	case domain.ScanStatusCancelled:
		return KindScanCancelled, "Scan cancelled"
	}
	return "", ""
}

func feedBody(info *ScanInfo) string {
	b := fmt.Sprintf("%s · Scan #%d", info.ProjectName, info.ScanNumber)
	if info.Score != nil && info.Verdict != nil {
		b += fmt.Sprintf(" — risk score %d (%s)", *info.Score, *info.Verdict)
	}
	if info.IsLiveScan() && info.TriggerRef != nil {
		b += " · live scan of " + *info.TriggerRef
	}
	return b
}

// GetSettings returns the actor's notification settings.
func (s *Service) GetSettings(ctx context.Context, actor domain.Actor) (*SettingsView, error) {
	settings, err := s.repo.GetSettings(ctx, actor.UserID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("load notification settings: %w", err))
	}
	accountEmail, _, err := s.repo.GetUserContact(ctx, actor.UserID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("load account email: %w", err))
	}
	view := &SettingsView{
		AccountEmail:        accountEmail,
		ReportEmail:         accountEmail,
		UsingAccountEmail:   true,
		EmailOnScanComplete: settings.EmailOnScanComplete,
		EmailOnLiveScan:     settings.EmailOnLiveScan,
		EmailEnabled:        s.cfg.EmailEnabled,
	}
	if settings.ReportEmail != nil {
		view.ReportEmail = *settings.ReportEmail
		view.UsingAccountEmail = false
	}
	// An expired pending address is just gone, as far as the user can tell.
	if settings.PendingEmail != nil && settings.PendingExpiresAt != nil && settings.PendingExpiresAt.After(s.now()) {
		view.PendingEmail = settings.PendingEmail
		view.PendingExpiresAt = settings.PendingExpiresAt
	}
	return view, nil
}

// UpdateSettings changes the actor's toggles and/or report address. A new
// address isn't used until verified: this sends the verification link and
// leaves reports going where they went before.
func (s *Service) UpdateSettings(ctx context.Context, actor domain.Actor, in UpdateSettingsInput) (*SettingsView, error) {
	current, err := s.repo.GetSettings(ctx, actor.UserID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("load notification settings: %w", err))
	}

	if in.EmailOnScanComplete != nil || in.EmailOnLiveScan != nil {
		onScan, onLive := current.EmailOnScanComplete, current.EmailOnLiveScan
		if in.EmailOnScanComplete != nil {
			onScan = *in.EmailOnScanComplete
		}
		if in.EmailOnLiveScan != nil {
			onLive = *in.EmailOnLiveScan
		}
		if err := s.repo.SaveToggles(ctx, actor.UserID, onScan, onLive); err != nil {
			return nil, apperrors.Internal(fmt.Errorf("save notification toggles: %w", err))
		}
	}

	if in.ReportEmail != nil {
		if err := s.changeReportEmail(ctx, actor, current, strings.TrimSpace(*in.ReportEmail)); err != nil {
			return nil, err
		}
	}
	return s.GetSettings(ctx, actor)
}

func (s *Service) changeReportEmail(ctx context.Context, actor domain.Actor, current Settings, requested string) error {
	accountEmail, displayName, err := s.repo.GetUserContact(ctx, actor.UserID)
	if err != nil {
		return apperrors.Internal(fmt.Errorf("load account email: %w", err))
	}

	if requested == "" || strings.EqualFold(requested, accountEmail) {
		if current.ReportEmail == nil && current.PendingEmail == nil {
			return nil
		}
		if err := s.repo.UseAccountEmail(ctx, actor.UserID); err != nil {
			return apperrors.Internal(fmt.Errorf("reset report email: %w", err))
		}
		s.logAudit(ctx, actor, "notification.report_email_reset", nil)
		return nil
	}
	if current.ReportEmail != nil && strings.EqualFold(requested, *current.ReportEmail) {
		return nil // already the verified address
	}

	addr, err := parseEmail(requested)
	if err != nil {
		return err
	}
	if !s.cfg.EmailEnabled {
		return apperrors.Unprocessable("notification.email_disabled", "Email delivery isn't configured on this server, so a report address can't be verified.")
	}
	return s.startVerification(ctx, actor, addr, displayName)
}

// ResendVerification sends a fresh verification link to the pending address.
func (s *Service) ResendVerification(ctx context.Context, actor domain.Actor) error {
	current, err := s.repo.GetSettings(ctx, actor.UserID)
	if err != nil {
		return apperrors.Internal(fmt.Errorf("load notification settings: %w", err))
	}
	if current.PendingEmail == nil {
		return apperrors.NotFound("notification.no_pending_email", "There's no report address waiting to be verified.")
	}
	if !s.cfg.EmailEnabled {
		return apperrors.Unprocessable("notification.email_disabled", "Email delivery isn't configured on this server.")
	}
	_, displayName, err := s.repo.GetUserContact(ctx, actor.UserID)
	if err != nil {
		return apperrors.Internal(fmt.Errorf("load account: %w", err))
	}
	return s.startVerification(ctx, actor, *current.PendingEmail, displayName)
}

func (s *Service) startVerification(ctx context.Context, actor domain.Actor, addr, displayName string) error {
	token, hash, err := newToken()
	if err != nil {
		return apperrors.Internal(err)
	}
	if err := s.repo.SetPendingEmail(ctx, actor.UserID, addr, hash, s.now().Add(verificationTTL)); err != nil {
		return apperrors.Internal(fmt.Errorf("save pending report email: %w", err))
	}
	link := s.cfg.AppURL + "/verify-report-email?token=" + token
	if err := s.mailer.Send(ctx, verificationEmail(addr, displayName, link)); err != nil {
		s.log.Error("notification: send verification email failed", "user_id", actor.UserID, "error", err)
		return apperrors.External("notification.send_failed", "The verification email couldn't be sent. Try again in a moment.", err)
	}
	s.logAudit(ctx, actor, "notification.report_email_requested", map[string]any{"email": addr})
	return nil
}

// ConfirmReportEmail completes verification from the emailed link. The
// token itself is the credential, so this needs no session — the link may
// well be opened on another device.
func (s *Service) ConfirmReportEmail(ctx context.Context, token string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != tokenBytes {
		return "", apperrors.NotFound("notification.invalid_token", "This verification link is invalid or has expired.")
	}
	sum := sha256.Sum256(raw)
	userID, email, err := s.repo.ConfirmPendingEmail(ctx, sum[:], s.now())
	if err != nil {
		var appErr *apperrors.Error
		if errors.As(err, &appErr) && appErr.Kind == apperrors.KindNotFound {
			return "", apperrors.NotFound("notification.invalid_token", "This verification link is invalid or has expired.")
		}
		return "", apperrors.Internal(fmt.Errorf("confirm report email: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{ActorID: &userID, Action: "notification.report_email_verified", Detail: map[string]any{"email": email}})
	return email, nil
}

// SendTest sends a short test email to wherever the actor's reports go now.
func (s *Service) SendTest(ctx context.Context, actor domain.Actor) (string, error) {
	if !s.cfg.EmailEnabled {
		return "", apperrors.Unprocessable("notification.email_disabled", "Email delivery isn't configured on this server.")
	}
	settings, err := s.repo.GetSettings(ctx, actor.UserID)
	if err != nil {
		return "", apperrors.Internal(fmt.Errorf("load notification settings: %w", err))
	}
	to, err := s.recipient(ctx, settings)
	if err != nil {
		return "", apperrors.Internal(err)
	}
	if err := s.mailer.Send(ctx, testEmail(to, s.cfg.AppURL)); err != nil {
		s.log.Error("notification: send test email failed", "user_id", actor.UserID, "error", err)
		return "", apperrors.External("notification.send_failed", "The test email couldn't be sent. Check the server's mail settings.", err)
	}
	return to, nil
}

// ListNotifications returns the actor's feed for their current org, newest
// first, plus how many are unread.
func (s *Service) ListNotifications(ctx context.Context, actor domain.Actor) ([]Notification, int, error) {
	items, unread, err := s.repo.ListNotifications(ctx, actor.UserID, actor.OrgID, feedLimit)
	if err != nil {
		return nil, 0, apperrors.Internal(fmt.Errorf("list notifications: %w", err))
	}
	return items, unread, nil
}

// MarkRead marks one of the actor's notifications read. Someone else's
// notification looks exactly like a missing one.
func (s *Service) MarkRead(ctx context.Context, actor domain.Actor, id uuid.UUID) error {
	return s.repo.MarkNotificationRead(ctx, actor.UserID, actor.OrgID, id)
}

// MarkAllRead marks every notification in the actor's current org read.
func (s *Service) MarkAllRead(ctx context.Context, actor domain.Actor) error {
	if err := s.repo.MarkAllNotificationsRead(ctx, actor.UserID, actor.OrgID); err != nil {
		return apperrors.Internal(fmt.Errorf("mark notifications read: %w", err))
	}
	return nil
}

func (s *Service) logAudit(ctx context.Context, actor domain.Actor, action string, detail map[string]any) {
	resType := "user"
	s.audit.Log(ctx, audit.Entry{
		OrgID: &actor.OrgID, ActorID: &actor.UserID, Action: action,
		ResourceType: &resType, ResourceID: &actor.UserID, Detail: detail,
	})
}

// maxEmailLength is RFC 5321's practical limit for a whole address.
const maxEmailLength = 254

// parseEmail accepts a bare address only ("a@b.co", not "Name <a@b.co>").
func parseEmail(in string) (string, error) {
	invalid := apperrors.Validation("notification.invalid_email", "Enter a valid email address.",
		[]apperrors.FieldError{{Field: "report_email", Message: "must be a valid email address"}})
	if len(in) > maxEmailLength {
		return "", invalid
	}
	addr, err := mail.ParseAddress(in)
	if err != nil || addr.Address != in || addr.Name != "" {
		return "", invalid
	}
	at := strings.LastIndex(in, "@")
	if at < 1 || !strings.Contains(in[at+1:], ".") {
		return "", invalid
	}
	return in, nil
}

const tokenBytes = 32

// newToken returns a random URL-safe token and the SHA-256 that's stored.
func newToken() (token string, hash []byte, err error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("generate verification token: %w", err)
	}
	sum := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(raw), sum[:], nil
}
