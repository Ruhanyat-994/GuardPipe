package notification

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Sender delivers queued report emails (scan_report_emails). Runs in the
// worker process (GUARDPIPE_ROLE=worker/all), next to the scan workers.
type Sender struct {
	Repo     Repository
	Mailer   Mailer
	Reports  ReportRenderer
	AppURL   string
	Log      *slog.Logger
	Interval time.Duration
	// MaxAttachmentBytes: a PDF larger than this is left out and the email
	// says to download it from the scan page instead. Amazon SES accepts
	// up to 40 MB per message, but many inboxes reject far less.
	MaxAttachmentBytes int
}

const (
	// senderBatch is how many emails one tick claims.
	senderBatch = 5
	// senderLease is how long a claimed email is reserved: if the process
	// dies mid-send, the email becomes due again after this.
	senderLease = 5 * time.Minute
	// maxAttempts — after this many failed tries the email is given up on
	// (status failed, last_error kept for the operator).
	maxAttempts = 6
)

// retryBackoff[n] is the wait after the (n+1)th failed attempt.
var retryBackoff = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 3 * time.Hour}

// Start polls for due emails until ctx is cancelled.
func (s *Sender) Start(ctx context.Context) {
	interval := s.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		s.RunOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce claims and delivers one batch. Exported for tests.
func (s *Sender) RunOnce(ctx context.Context) {
	emails, err := s.Repo.ClaimDueEmails(ctx, senderBatch, senderLease)
	if err != nil {
		s.Log.Error("notification: claim report emails failed", "error", err)
		return
	}
	for _, e := range emails {
		if ctx.Err() != nil {
			return // shutting down — the lease makes these due again later
		}
		s.deliver(ctx, e)
	}
}

func (s *Sender) deliver(ctx context.Context, e OutboxEmail) {
	err := s.send(ctx, e)
	if err == nil {
		if markErr := s.Repo.MarkEmailSent(ctx, e.ID); markErr != nil {
			s.Log.Error("notification: mark report email sent failed", "email_id", e.ID, "error", markErr)
		}
		s.Log.Info("notification: report email sent", "scan_id", e.ScanID, "user_id", e.UserID)
		return
	}

	msg := truncate(err.Error(), 500)
	if e.Attempts >= maxAttempts {
		s.Log.Error("notification: report email given up", "scan_id", e.ScanID, "attempts", e.Attempts, "error", err)
		if markErr := s.Repo.MarkEmailFailed(ctx, e.ID, msg); markErr != nil {
			s.Log.Error("notification: mark report email failed", "email_id", e.ID, "error", markErr)
		}
		return
	}
	wait := retryBackoff[min(e.Attempts-1, len(retryBackoff)-1)]
	s.Log.Warn("notification: report email failed, will retry", "scan_id", e.ScanID, "attempt", e.Attempts, "retry_in", wait, "error", err)
	if markErr := s.Repo.MarkEmailRetry(ctx, e.ID, time.Now().Add(wait), msg); markErr != nil {
		s.Log.Error("notification: schedule report email retry failed", "email_id", e.ID, "error", markErr)
	}
}

func (s *Sender) send(ctx context.Context, e OutboxEmail) error {
	info, err := s.Repo.GetScanInfo(ctx, e.ScanID)
	if err != nil {
		return fmt.Errorf("load scan: %w", err)
	}
	pdf, err := s.Reports.RenderScanPDF(ctx, e.OrgID, e.ScanID)
	if err != nil {
		return fmt.Errorf("render report: %w", err)
	}
	attach := s.MaxAttachmentBytes <= 0 || len(pdf) <= s.MaxAttachmentBytes
	link := fmt.Sprintf("%s/scans/%s", trimSlash(s.AppURL), e.ScanID)
	email, err := reportEmail(e.Recipient, info, link, pdf, attach)
	if err != nil {
		return err
	}
	return s.Mailer.Send(ctx, email)
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
