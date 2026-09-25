package notification_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/notification"
)

func newSender(h *harness, renderer notification.ReportRenderer, maxAttach int) *notification.Sender {
	return &notification.Sender{
		Repo: h.repo, Mailer: h.mailer, Reports: renderer, AppURL: "https://app.example.com",
		Log: slog.New(slog.DiscardHandler), MaxAttachmentBytes: maxAttach,
	}
}

func TestSender_DeliversReportWithPDFAttached(t *testing.T) {
	h := newHarness(t, true)
	scanID := h.addScan(domain.ScanStatusCompleted, domain.TriggerManual)
	require.NoError(t, h.svc.ScanFinished(context.Background(), scanID))

	newSender(h, fakeRenderer{pdf: []byte("%PDF-1.7 fake")}, 1<<20).RunOnce(context.Background())

	require.Len(t, h.mailer.sent, 1)
	e := h.mailer.sent[0]
	require.Equal(t, "owner@example.com", e.To)
	require.Contains(t, e.Subject, "Payments API · Scan #4")
	require.Contains(t, e.Subject, "risk score 72 (warn)")
	require.Contains(t, e.Text, "https://app.example.com/scans/"+scanID.String())
	require.Len(t, e.Attachments, 1)
	require.Equal(t, "application/pdf", e.Attachments[0].ContentType)
	require.Equal(t, "guardpipe-scan-4.pdf", e.Attachments[0].Filename)
	require.Equal(t, "sent", h.repo.outboxStatus[h.repo.outbox[0].ID])

	// Already sent — a second pass sends nothing more.
	newSender(h, fakeRenderer{pdf: []byte("x")}, 1<<20).RunOnce(context.Background())
	require.Len(t, h.mailer.sent, 1)
}

func TestSender_OversizedPDF_LinkedNotAttached(t *testing.T) {
	h := newHarness(t, true)
	require.NoError(t, h.svc.ScanFinished(context.Background(), h.addScan(domain.ScanStatusCompleted, domain.TriggerManual)))

	newSender(h, fakeRenderer{pdf: make([]byte, 2048)}, 1024).RunOnce(context.Background())

	require.Len(t, h.mailer.sent, 1)
	require.Empty(t, h.mailer.sent[0].Attachments)
	require.Contains(t, h.mailer.sent[0].Text, "too large to attach")
}

func TestSender_FailureIsRetriedThenGivenUp(t *testing.T) {
	h := newHarness(t, true)
	require.NoError(t, h.svc.ScanFinished(context.Background(), h.addScan(domain.ScanStatusCompleted, domain.TriggerManual)))
	h.mailer.err = errors.New("ses throttled")
	id := h.repo.outbox[0].ID

	s := newSender(h, fakeRenderer{pdf: []byte("x")}, 1<<20)
	s.RunOnce(context.Background())
	require.Equal(t, "pending", h.repo.outboxStatus[id], "a failed send is rescheduled, not dropped")

	// Force it due again until attempts run out.
	for i := 0; i < 10 && h.repo.outboxStatus[id] != "failed"; i++ {
		delete(h.repo.nextAttempt, id)
		s.RunOnce(context.Background())
	}
	require.Equal(t, "failed", h.repo.outboxStatus[id])
	require.Equal(t, 6, h.repo.outbox[0].Attempts)
}

func TestReportEmail_EscapesProjectNameInHTML(t *testing.T) {
	h := newHarness(t, true)
	id := h.addScan(domain.ScanStatusFailed, domain.TriggerManual)
	h.repo.scans[id].ProjectName = `<img src=x onerror=alert(1)>`
	h.repo.scans[id].Score, h.repo.scans[id].Verdict = nil, nil
	require.NoError(t, h.svc.ScanFinished(context.Background(), id))

	newSender(h, fakeRenderer{pdf: []byte("x")}, 1<<20).RunOnce(context.Background())

	require.Len(t, h.mailer.sent, 1)
	e := h.mailer.sent[0]
	require.NotContains(t, e.HTML, "<img src=x")
	require.Contains(t, e.HTML, "&lt;img src=x")
	require.Contains(t, e.Subject, "scan failed")
	require.False(t, strings.ContainsAny(e.Subject, "\r\n"))
}
