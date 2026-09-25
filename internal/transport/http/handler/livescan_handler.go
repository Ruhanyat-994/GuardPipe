package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/livescan"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/dto"
)

// maxWebhookBody caps what the receiver reads. GitHub's own ceiling is
// 25 MB, but a push payload that large is a monorepo-sized edge case; a
// few MB is plenty for real pushes and pull requests.
const maxWebhookBody = 5 << 20

// UserNameLookup resolves a user's display name for "enabled by" — the
// same *store/repo.UserRepo the report assembler already uses.
type UserNameLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (*identity.User, error)
}

// LiveScanHandler serves GitHub webhook live scanning (BUILD_GUIDE.md
// Phase 17 Part B): the per-project settings endpoints, and the webhook
// receiver GitHub itself calls.
type LiveScanHandler struct {
	svc       livescan.Service
	users     UserNameLookup
	validator *validate.Validator
}

func NewLiveScanHandler(svc livescan.Service, users UserNameLookup, validator *validate.Validator) *LiveScanHandler {
	return &LiveScanHandler{svc: svc, users: users, validator: validator}
}

// Get handles `GET /projects/{id}/live-scanning`.
func (h *LiveScanHandler) Get(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	projectID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	w, err := h.svc.Get(c.Request.Context(), actor, projectID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromLiveScan(w, h.enabledByName(c.Request.Context(), w)))
}

// Enable handles `PUT /projects/{id}/live-scanning` — turn on, or change
// settings / resume after a pause.
func (h *LiveScanHandler) Enable(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	projectID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	var req dto.EnableLiveScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.Validation("livescan.invalid_body", "request body could not be parsed", nil))
		return
	}
	if fieldErrs := h.validator.Struct(req); len(fieldErrs) > 0 {
		c.Error(apperrors.Validation("livescan.invalid_input", "one or more fields are invalid", toAppFieldErrors(fieldErrs)))
		return
	}
	w, err := h.svc.Enable(c.Request.Context(), actor, projectID, req.ToInput())
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromLiveScan(w, h.enabledByName(c.Request.Context(), w)))
}

// Disable handles `DELETE /projects/{id}/live-scanning`.
func (h *LiveScanHandler) Disable(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	projectID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	res, err := h.svc.Disable(c.Request.Context(), actor, projectID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.DisableLiveScanResponse{GitHubHookRemoved: res.GitHubHookRemoved})
}

// Receive handles `POST /webhooks/github/{id}` — called by GitHub, not a
// user, so it sits outside the JWT/RBAC chain and is authenticated by the
// X-Hub-Signature-256 HMAC instead (checked in livescan.Service.Receive).
// Answers 202 as soon as the event is verified and queued.
func (h *LiveScanHandler) Receive(c *gin.Context) {
	webhookID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Error(apperrors.NotFound("webhook.not_found", "unknown webhook"))
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxWebhookBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.Error(apperrors.Validation("webhook.payload_too_large", "webhook payload is too large", nil))
			return
		}
		c.Error(apperrors.Validation("webhook.invalid_body", "could not read webhook payload", nil))
		return
	}
	err = h.svc.Receive(c.Request.Context(), livescan.Delivery{
		WebhookID:  webhookID,
		Event:      c.GetHeader("X-GitHub-Event"),
		DeliveryID: c.GetHeader("X-GitHub-Delivery"),
		Signature:  c.GetHeader("X-Hub-Signature-256"),
		Body:       body,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusAccepted)
}

func (h *LiveScanHandler) enabledByName(ctx context.Context, w *livescan.Webhook) string {
	if w == nil || w.EnabledBy == nil || h.users == nil {
		return ""
	}
	u, err := h.users.GetByID(ctx, *w.EnabledBy)
	if err != nil || u == nil {
		return ""
	}
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Email
}
