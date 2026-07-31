// Package handler holds the HTTP handlers. Every handler is bind-validate,
// call the service, delegate errors, render a DTO — nothing else
// (documentation/04-backend-architecture.md §4.2). Handlers never call
// repositories and never build error JSON themselves.
package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/dto"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/middleware"
)

// refreshCookieName is `gp_refresh` per documentation/07-api-specification.md
// §2.
const refreshCookieName = "gp_refresh"

// refreshCookiePath scopes the cookie to only the endpoints that need it,
// so it isn't sent on every single API request.
const refreshCookiePath = "/api/v1/auth"

// AuthHandler implements the 5 endpoints in documentation/07-api-specification.md
// §2.
type AuthHandler struct {
	svc             identity.Service
	validator       *validate.Validator
	secureCookies   bool
	refreshTokenTTL time.Duration
}

// NewAuthHandler builds an AuthHandler. secureCookies should be true in
// production (HTTPS) and false in local development — a browser refuses to
// send a `Secure` cookie back over plain HTTP, which would break local
// `npm run dev` testing entirely if hardcoded true
// (documentation/07-api-specification.md's cookie is written assuming an
// HTTPS deployment; this parameter is the documented deviation for local
// dev, see PROGRESS-LOG.md).
func NewAuthHandler(svc identity.Service, validator *validate.Validator, secureCookies bool, refreshTokenTTL time.Duration) *AuthHandler {
	return &AuthHandler{svc: svc, validator: validator, secureCookies: secureCookies, refreshTokenTTL: refreshTokenTTL}
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.Validation("identity.invalid_body", "request body could not be parsed", nil))
		return
	}
	if fieldErrs := h.validator.Struct(req); len(fieldErrs) > 0 {
		c.Error(apperrors.Validation("identity.invalid_input", "one or more fields are invalid", toAppFieldErrors(fieldErrs)))
		return
	}

	user, err := h.svc.Register(c.Request.Context(), req.ToInput())
	if err != nil {
		c.Error(err)
		return
	}

	c.Header("Location", "/api/v1/auth/me")
	c.JSON(http.StatusCreated, dto.FromUser(user))
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.Validation("identity.invalid_body", "request body could not be parsed", nil))
		return
	}

	pair, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.Error(err)
		return
	}

	h.setRefreshCookie(c, pair.RefreshToken)
	c.JSON(http.StatusOK, dto.LoginResponse{
		AccessToken: pair.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   pair.ExpiresIn,
		User:        dto.FromUser(pair.User),
	})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie(refreshCookieName)
	if err != nil || refreshToken == "" {
		c.Error(apperrors.Unauthorized("auth.token_invalid", "no refresh token cookie present"))
		return
	}

	pair, err := h.svc.Refresh(c.Request.Context(), refreshToken)
	if err != nil {
		h.clearRefreshCookie(c)
		c.Error(err)
		return
	}

	h.setRefreshCookie(c, pair.RefreshToken)
	c.JSON(http.StatusOK, dto.RefreshResponse{
		AccessToken: pair.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   pair.ExpiresIn,
	})
}

// Logout is idempotent — presenting no cookie, or a cookie for an
// already-dead session, is still a 204, not an error
// (documentation/05-module-specifications.md §3's Service.Logout has the
// same contract).
func (h *AuthHandler) Logout(c *gin.Context) {
	if refreshToken, err := c.Cookie(refreshCookieName); err == nil && refreshToken != "" {
		_ = h.svc.Logout(c.Request.Context(), refreshToken)
	}
	h.clearRefreshCookie(c)
	c.Status(http.StatusNoContent)
}

func (h *AuthHandler) Me(c *gin.Context) {
	actor, ok := middleware.ActorFromContext(c)
	if !ok {
		c.Error(apperrors.Internal(errors.New("Me handler reached without an authenticated actor")))
		return
	}

	user, err := h.svc.Me(c.Request.Context(), actor)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromUser(user))
}

func (h *AuthHandler) setRefreshCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, token, int(h.refreshTokenTTL.Seconds()), refreshCookiePath, "", h.secureCookies, true)
}

func (h *AuthHandler) clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, "", -1, refreshCookiePath, "", h.secureCookies, true)
}

func toAppFieldErrors(fieldErrs []validate.FieldError) []apperrors.FieldError {
	out := make([]apperrors.FieldError, len(fieldErrs))
	for i, fe := range fieldErrs {
		out[i] = apperrors.FieldError{Field: fe.Field, Message: fe.Message}
	}
	return out
}
