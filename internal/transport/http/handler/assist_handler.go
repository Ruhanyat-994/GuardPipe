package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/assist"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/billing"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/dto"
)

// AssistHandler serves the finding assistant's commands.
type AssistHandler struct {
	svc       *assist.Service
	validator *validate.Validator
}

func NewAssistHandler(svc *assist.Service, validator *validate.Validator) *AssistHandler {
	return &AssistHandler{svc: svc, validator: validator}
}

// Run handles `POST /findings/{id}/assist` — one command about one finding.
func (h *AssistHandler) Run(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	findingID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	var req dto.AssistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.Validation("assist.invalid_body", "request body could not be parsed", nil))
		return
	}
	if fieldErrs := h.validator.Struct(req); len(fieldErrs) > 0 {
		c.Error(apperrors.Validation("assist.invalid_input", "action must be one of explain, remediate, fix", toAppFieldErrors(fieldErrs)))
		return
	}
	reply, err := h.svc.Run(c.Request.Context(), actor, findingID, billing.AIAction(req.Action))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromAssistReply(reply))
}
