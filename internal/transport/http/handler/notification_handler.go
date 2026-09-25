package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/notification"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/dto"
)

// NotificationHandler serves the notification feed (`/notifications`) and a
// user's own scan-report email settings (`/me/notification-settings`).
// Every route acts on the caller's own data only — there's no user id in
// any path to tamper with.
type NotificationHandler struct {
	svc       *notification.Service
	validator *validate.Validator
}

func NewNotificationHandler(svc *notification.Service, validator *validate.Validator) *NotificationHandler {
	return &NotificationHandler{svc: svc, validator: validator}
}

func (h *NotificationHandler) bind(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		c.Error(apperrors.Validation("notification.invalid_body", "request body could not be parsed", nil))
		return false
	}
	if fieldErrs := h.validator.Struct(req); len(fieldErrs) > 0 {
		c.Error(apperrors.Validation("notification.invalid_input", "one or more fields are invalid", toAppFieldErrors(fieldErrs)))
		return false
	}
	return true
}

// GetSettings handles `GET /me/notification-settings`.
func (h *NotificationHandler) GetSettings(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	view, err := h.svc.GetSettings(c.Request.Context(), actor)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromNotificationSettings(view))
}

// UpdateSettings handles `PUT /me/notification-settings`.
func (h *NotificationHandler) UpdateSettings(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	var req dto.UpdateNotificationSettingsRequest
	if !h.bind(c, &req) {
		return
	}
	view, err := h.svc.UpdateSettings(c.Request.Context(), actor, notification.UpdateSettingsInput{
		EmailOnScanComplete: req.EmailOnScanComplete,
		EmailOnLiveScan:     req.EmailOnLiveScan,
		ReportEmail:         req.ReportEmail,
	})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromNotificationSettings(view))
}

// ResendVerification handles `POST /me/notification-settings/resend-verification`.
func (h *NotificationHandler) ResendVerification(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	if err := h.svc.ResendVerification(c.Request.Context(), actor); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

// SendTest handles `POST /me/notification-settings/test`.
func (h *NotificationHandler) SendTest(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	to, err := h.svc.SendTest(c.Request.Context(), actor)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.TestEmailResponse{SentTo: to})
}

// VerifyReportEmail handles `POST /notification-settings/verify` — public:
// the emailed token is the credential, and the link may be opened on a
// device with no GuardPipe session.
func (h *NotificationHandler) VerifyReportEmail(c *gin.Context) {
	var req dto.VerifyReportEmailRequest
	if !h.bind(c, &req) {
		return
	}
	email, err := h.svc.ConfirmReportEmail(c.Request.Context(), req.Token)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.VerifyReportEmailResponse{ReportEmail: email})
}

// List handles `GET /notifications`.
func (h *NotificationHandler) List(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	items, unread, err := h.svc.ListNotifications(c.Request.Context(), actor)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromNotifications(items, unread))
}

// MarkRead handles `POST /notifications/{id}/read`.
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	id, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), actor, id); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

// MarkAllRead handles `POST /notifications/read-all`.
func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	if err := h.svc.MarkAllRead(c.Request.Context(), actor); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}
