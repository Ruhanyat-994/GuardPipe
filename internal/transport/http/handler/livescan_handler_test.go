package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/livescan"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/dto"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/handler"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/middleware"
)

// fakeLiveScanService is a hand-written fake — the service's own behaviour
// is covered by internal/modules/livescan's tests; these are about HTTP.
type fakeLiveScanService struct {
	webhook  *livescan.Webhook
	enableIn livescan.EnableInput
	delivery livescan.Delivery
	err      error
	removed  bool
}

func (f *fakeLiveScanService) Get(context.Context, domain.Actor, uuid.UUID) (*livescan.Webhook, error) {
	return f.webhook, f.err
}
func (f *fakeLiveScanService) Enable(_ context.Context, _ domain.Actor, _ uuid.UUID, in livescan.EnableInput) (*livescan.Webhook, error) {
	f.enableIn = in
	return f.webhook, f.err
}
func (f *fakeLiveScanService) Disable(context.Context, domain.Actor, uuid.UUID) (*livescan.DisableResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &livescan.DisableResult{GitHubHookRemoved: f.removed}, nil
}
func (f *fakeLiveScanService) Receive(_ context.Context, d livescan.Delivery) error {
	f.delivery = d
	return f.err
}
func (f *fakeLiveScanService) HandleQueuedEvent(context.Context, []byte) error { return nil }
func (f *fakeLiveScanService) FireDue(context.Context, time.Time) error        { return nil }

func newLiveScanRouter(svc livescan.Service) *gin.Engine {
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	h := handler.NewLiveScanHandler(svc, nil, validate.New())
	// The receiver has no actor — exactly like production, where it's
	// registered outside the auth chain.
	r.POST("/webhooks/github/:id", h.Receive)
	authed := r.Group("", func(c *gin.Context) {
		c.Set("actor", domain.Actor{UserID: id.New(), OrgID: id.New(), Role: domain.RoleAdmin})
		c.Next()
	})
	authed.GET("/projects/:id/live-scanning", h.Get)
	authed.PUT("/projects/:id/live-scanning", h.Enable)
	authed.DELETE("/projects/:id/live-scanning", h.Disable)
	return r
}

func TestLiveScanReceive_PassesHeadersAndRawBodyThrough_Returns202(t *testing.T) {
	svc := &fakeLiveScanService{}
	r := newLiveScanRouter(svc)
	hookID := id.New()
	body := []byte(`{"ref":"refs/heads/main"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github/"+hookID.String(), bytes.NewReader(body))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-GitHub-Delivery", "delivery-123")
	req.Header.Set("X-Hub-Signature-256", "sha256=abc")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body: %s", rec.Code, rec.Body.String())
	}
	if svc.delivery.WebhookID != hookID || svc.delivery.Event != "push" || svc.delivery.DeliveryID != "delivery-123" ||
		svc.delivery.Signature != "sha256=abc" || !bytes.Equal(svc.delivery.Body, body) {
		t.Fatalf("delivery not passed through intact: %+v", svc.delivery)
	}
}

func TestLiveScanReceive_BadSignature_Returns401(t *testing.T) {
	svc := &fakeLiveScanService{err: apperrors.Unauthorized("webhook.signature_invalid", "webhook signature verification failed")}
	r := newLiveScanRouter(svc)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github/"+id.New().String(), strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestLiveScanReceive_NonUUIDPath_Returns404WithoutCallingService(t *testing.T) {
	svc := &fakeLiveScanService{}
	r := newLiveScanRouter(svc)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github/not-a-uuid", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if svc.delivery.Body != nil {
		t.Fatal("service must not be called for a malformed webhook id")
	}
}

func TestLiveScanGet_Off_ReportsDisabledAndNeverOffersPentest(t *testing.T) {
	r := newLiveScanRouter(&fakeLiveScanService{})
	rec := doJSON(t, r, http.MethodGet, "/projects/"+id.New().String()+"/live-scanning", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp dto.LiveScanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Enabled {
		t.Fatal("expected enabled=false")
	}
	for _, e := range resp.AllowedEngines {
		if e == "pentest" {
			t.Fatal("pentest must never be offered for automatic scans")
		}
	}
}

func TestLiveScanEnable_ResponseNeverContainsTheSecret(t *testing.T) {
	enabledBy := id.New()
	svc := &fakeLiveScanService{webhook: &livescan.Webhook{
		ID: id.New(), Engines: []domain.EngineID{domain.EngineCodeScan}, WatchedBranches: []string{"main"},
		SecretCiphertext: []byte("CIPHERTEXT-MARKER"), SecretNonce: []byte("NONCE-MARKER"),
		EnabledBy: &enabledBy, AttestedAt: time.Now(),
	}}
	r := newLiveScanRouter(svc)
	rec := doJSON(t, r, http.MethodPut, "/projects/"+id.New().String()+"/live-scanning", map[string]any{
		"engines": []string{"codescan"}, "watched_branches": []string{"main"}, "confirmed": true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "CIPHERTEXT") || strings.Contains(body, "NONCE") || strings.Contains(body, "secret") {
		t.Fatalf("response leaks secret material: %s", body)
	}
	if !svc.enableIn.Confirmed || len(svc.enableIn.Engines) != 1 {
		t.Fatalf("input not passed through: %+v", svc.enableIn)
	}
}

func TestLiveScanEnable_UnknownEngine_Returns400(t *testing.T) {
	r := newLiveScanRouter(&fakeLiveScanService{})
	rec := doJSON(t, r, http.MethodPut, "/projects/"+id.New().String()+"/live-scanning", map[string]any{
		"engines": []string{"nmap"}, "confirmed": true,
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestLiveScanDisable_ReportsWhetherHookWasRemoved(t *testing.T) {
	r := newLiveScanRouter(&fakeLiveScanService{removed: false})
	rec := doJSON(t, r, http.MethodDelete, "/projects/"+id.New().String()+"/live-scanning", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"github_hook_removed":false`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestScanResponse_CarriesTriggerFields(t *testing.T) {
	detail := sampleScanDetail()
	ref, actor := "main", "octocat"
	detail.TriggerSource, detail.TriggerRef, detail.TriggerActor = domain.TriggerWebhookPush, &ref, &actor
	r := newScanRouter(&fakeOrchestratorService{detail: detail})

	rec := doJSON(t, r, http.MethodGet, "/scans/"+detail.ID.String(), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	for _, want := range []string{`"trigger_source":"webhook_push"`, `"trigger_ref":"main"`, `"trigger_actor":"octocat"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("missing %s in %s", want, rec.Body.String())
		}
	}
}
