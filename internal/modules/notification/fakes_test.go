package notification_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/notification"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

type fakeRepo struct {
	mu            sync.Mutex
	settings      map[uuid.UUID]notification.Settings
	pendingHashes map[uuid.UUID][]byte
	users         map[uuid.UUID][2]string
	scans         map[uuid.UUID]*notification.ScanInfo
	outbox        []notification.OutboxEmail
	outboxStatus  map[uuid.UUID]string
	nextAttempt   map[uuid.UUID]time.Time
	feed          []notification.Notification
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		settings:      map[uuid.UUID]notification.Settings{},
		pendingHashes: map[uuid.UUID][]byte{},
		users:         map[uuid.UUID][2]string{},
		scans:         map[uuid.UUID]*notification.ScanInfo{},
		outboxStatus:  map[uuid.UUID]string{},
		nextAttempt:   map[uuid.UUID]time.Time{},
	}
}

func (f *fakeRepo) GetSettings(_ context.Context, userID uuid.UUID) (notification.Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.settings[userID]; ok {
		return s, nil
	}
	return notification.DefaultSettings(userID), nil
}

func (f *fakeRepo) SaveToggles(ctx context.Context, userID uuid.UUID, onScan, onLive bool) error {
	s, _ := f.GetSettings(ctx, userID)
	f.mu.Lock()
	defer f.mu.Unlock()
	s.EmailOnScanComplete, s.EmailOnLiveScan = onScan, onLive
	f.settings[userID] = s
	return nil
}

func (f *fakeRepo) SetPendingEmail(ctx context.Context, userID uuid.UUID, email string, hash []byte, exp time.Time) error {
	s, _ := f.GetSettings(ctx, userID)
	f.mu.Lock()
	defer f.mu.Unlock()
	s.PendingEmail, s.PendingExpiresAt = &email, &exp
	f.settings[userID] = s
	f.pendingHashes[userID] = hash
	return nil
}

func (f *fakeRepo) UseAccountEmail(ctx context.Context, userID uuid.UUID) error {
	s, _ := f.GetSettings(ctx, userID)
	f.mu.Lock()
	defer f.mu.Unlock()
	s.ReportEmail, s.PendingEmail, s.PendingExpiresAt = nil, nil, nil
	f.settings[userID] = s
	delete(f.pendingHashes, userID)
	return nil
}

func (f *fakeRepo) ConfirmPendingEmail(_ context.Context, hash []byte, now time.Time) (uuid.UUID, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for userID, h := range f.pendingHashes {
		s := f.settings[userID]
		if bytes.Equal(h, hash) && s.PendingExpiresAt != nil && s.PendingExpiresAt.After(now) {
			email := *s.PendingEmail
			s.ReportEmail, s.PendingEmail, s.PendingExpiresAt = &email, nil, nil
			f.settings[userID] = s
			delete(f.pendingHashes, userID)
			return userID, email, nil
		}
	}
	return uuid.Nil, "", apperrors.NotFound("notification.invalid_token", "not found")
}

func (f *fakeRepo) GetUserContact(_ context.Context, userID uuid.UUID) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[userID]
	if !ok {
		return "", "", apperrors.NotFound("user.not_found", "not found")
	}
	return u[0], u[1], nil
}

func (f *fakeRepo) GetScanInfo(_ context.Context, scanID uuid.UUID) (*notification.ScanInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.scans[scanID]
	if !ok {
		return nil, apperrors.NotFound("scan.not_found", "not found")
	}
	cp := *s
	return &cp, nil
}

func (f *fakeRepo) EnqueueReportEmail(_ context.Context, e notification.OutboxEmail) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.outbox {
		if existing.ScanID == e.ScanID && existing.UserID == e.UserID {
			return nil
		}
	}
	e.ID = uuid.New()
	f.outbox = append(f.outbox, e)
	f.outboxStatus[e.ID] = "pending"
	return nil
}

func (f *fakeRepo) ClaimDueEmails(_ context.Context, limit int, lease time.Duration) ([]notification.OutboxEmail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []notification.OutboxEmail
	for i := range f.outbox {
		e := &f.outbox[i]
		st := f.outboxStatus[e.ID]
		if (st == "pending" || st == "sending") && !f.nextAttempt[e.ID].After(time.Now()) && len(out) < limit {
			e.Attempts++
			f.outboxStatus[e.ID] = "sending"
			f.nextAttempt[e.ID] = time.Now().Add(lease)
			out = append(out, *e)
		}
	}
	return out, nil
}

func (f *fakeRepo) MarkEmailSent(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outboxStatus[id] = "sent"
	return nil
}

func (f *fakeRepo) MarkEmailRetry(_ context.Context, id uuid.UUID, next time.Time, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outboxStatus[id] = "pending"
	f.nextAttempt[id] = next
	return nil
}

func (f *fakeRepo) MarkEmailFailed(_ context.Context, id uuid.UUID, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.outboxStatus[id] = "failed"
	return nil
}

func (f *fakeRepo) CreateNotification(_ context.Context, n notification.Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.feed {
		if existing.ScanID != nil && n.ScanID != nil && *existing.ScanID == *n.ScanID && existing.UserID == n.UserID {
			return nil
		}
	}
	n.ID = uuid.New()
	f.feed = append(f.feed, n)
	return nil
}

func (f *fakeRepo) ListNotifications(_ context.Context, userID, orgID uuid.UUID, _ int) ([]notification.Notification, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []notification.Notification
	unread := 0
	for _, n := range f.feed {
		if n.UserID == userID && n.OrgID == orgID {
			out = append(out, n)
			if n.ReadAt == nil {
				unread++
			}
		}
	}
	return out, unread, nil
}

func (f *fakeRepo) MarkNotificationRead(_ context.Context, userID, orgID, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.feed {
		n := &f.feed[i]
		if n.ID == id && n.UserID == userID && n.OrgID == orgID {
			now := time.Now()
			n.ReadAt = &now
			return nil
		}
	}
	return apperrors.NotFound("notification.not_found", "not found")
}

func (f *fakeRepo) MarkAllNotificationsRead(_ context.Context, userID, orgID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for i := range f.feed {
		if f.feed[i].UserID == userID && f.feed[i].OrgID == orgID {
			f.feed[i].ReadAt = &now
		}
	}
	return nil
}

type fakeMailer struct {
	mu   sync.Mutex
	sent []notification.Email
	err  error
}

func (m *fakeMailer) Send(_ context.Context, e notification.Email) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, e)
	return nil
}

type fakeRenderer struct {
	pdf []byte
	err error
}

func (r fakeRenderer) RenderScanPDF(context.Context, uuid.UUID, uuid.UUID) ([]byte, error) {
	return r.pdf, r.err
}

type fakeAudit struct {
	mu      sync.Mutex
	actions []string
}

func (a *fakeAudit) Log(_ context.Context, e audit.Entry) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.actions = append(a.actions, e.Action)
}

func (a *fakeAudit) List(context.Context, audit.ListFilter, audit.Page) ([]audit.Entry, int, error) {
	return nil, 0, errors.New("not implemented")
}
