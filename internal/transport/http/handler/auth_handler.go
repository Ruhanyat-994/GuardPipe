// Package handler holds the HTTP handlers. Every handler is bind-validate,
// call the service, delegate errors, render a DTO — nothing else
// (documentation/04-backend-architecture.md §4.2). Handlers never call
// repositories and never build error JSON themselves.
package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/organization"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/dto"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/middleware"
)

// OperatorLookup is the narrow slice of admin.Service AuthHandler needs —
// just enough to stamp is_platform_operator onto a UserResponse
// (BUILD_GUIDE.md Phase 14), not admin's whole surface.
type OperatorLookup interface {
	IsOperator(ctx context.Context, userID uuid.UUID) (bool, error)
}

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
	operators       OperatorLookup
	// orgSvc backs the one BUILD_GUIDE.md Phase 15 addition to this
	// handler — SwitchOrg (`POST /auth/switch-org`) — kept here rather than
	// on a separate OrganizationHandler because it needs the exact same
	// refresh-cookie machinery (setRefreshCookie) Login/Refresh already
	// have; may be nil in a test that never exercises SwitchOrg.
	orgSvc organization.Service
	// projectSvc backs the project-collaborators follow-up's own addition
	// to this handler — SwitchProject (`POST /auth/switch-project/{id}`) —
	// kept here for the exact same reason orgSvc is: it needs
	// setRefreshCookie. May be nil in a test that never exercises it.
	projectSvc      project.Service
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
// dev, see PROGRESS-LOG.md). operators is never nil in production
// (cmd/guardpipe/main.go always wires modules/admin); orgSvc likewise
// (modules/organization, BUILD_GUIDE.md Phase 15).
func NewAuthHandler(svc identity.Service, operators OperatorLookup, orgSvc organization.Service, projectSvc project.Service, validator *validate.Validator, secureCookies bool, refreshTokenTTL time.Duration) *AuthHandler {
	return &AuthHandler{svc: svc, operators: operators, orgSvc: orgSvc, projectSvc: projectSvc, validator: validator, secureCookies: secureCookies, refreshTokenTTL: refreshTokenTTL}
}

// isOperator is a small helper so Login/Me don't each repeat the same
// nil-check/error-swallow — a lookup failure degrades to "not an operator"
// rather than failing the whole login/me call over a cosmetic UX flag.
func (h *AuthHandler) isOperator(ctx context.Context, userID uuid.UUID) bool {
	if h.operators == nil {
		return false
	}
	ok, err := h.operators.IsOperator(ctx, userID)
	return err == nil && ok
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
	// A freshly registered user can never already be a platform operator —
	// operators.Grant requires an existing user (cmd/guardpipe/admin.go's
	// own doc comment) — so this is always false, no lookup needed.
	c.JSON(http.StatusCreated, dto.FromUser(user, false))
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
		User:        dto.FromUser(pair.User, h.isOperator(c.Request.Context(), pair.User.ID)),
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
	c.JSON(http.StatusOK, dto.FromUserInOrg(user, actor.OrgID, actor.Role, actor.ProjectID, h.isOperator(c.Request.Context(), user.ID)))
}

// SwitchOrg is `POST /auth/switch-org` (BUILD_GUIDE.md Phase 15) — re-issues
// a token pair scoped to a different organisation the caller already holds
// a membership in (organization.Service.SwitchOrg does the actual
// membership check); `403` for any org they don't.
func (h *AuthHandler) SwitchOrg(c *gin.Context) {
	var req dto.SwitchOrgRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(apperrors.Validation("identity.invalid_body", "request body could not be parsed", nil))
		return
	}
	if fieldErrs := h.validator.Struct(req); len(fieldErrs) > 0 {
		c.Error(apperrors.Validation("identity.invalid_input", "one or more fields are invalid", toAppFieldErrors(fieldErrs)))
		return
	}
	orgID, err := uuid.Parse(req.OrgID)
	if err != nil {
		c.Error(apperrors.Validation("identity.invalid_input", "org_id is not a valid UUID", nil))
		return
	}
	actor, ok := middleware.ActorFromContext(c)
	if !ok {
		c.Error(apperrors.Internal(errors.New("SwitchOrg handler reached without an authenticated actor")))
		return
	}

	result, err := h.orgSvc.SwitchOrg(c.Request.Context(), actor, orgID)
	if err != nil {
		c.Error(err)
		return
	}

	h.setRefreshCookie(c, result.RefreshToken)
	c.JSON(http.StatusOK, dto.SwitchOrgResponse{
		AccessToken: result.AccessToken, TokenType: "Bearer", ExpiresIn: result.ExpiresIn,
	})
}

// SwitchProject is `POST /auth/switch-project/{id}` (project-collaborators
// follow-up) — re-issues a token pair scoped to exactly one project the
// caller holds an accepted collaborator grant on
// (project.Service.SwitchProject does the actual grant check); `403` for
// any project they don't.
func (h *AuthHandler) SwitchProject(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.Error(apperrors.Validation("identity.invalid_input", "id is not a valid UUID", nil))
		return
	}
	actor, ok := middleware.ActorFromContext(c)
	if !ok {
		c.Error(apperrors.Internal(errors.New("SwitchProject handler reached without an authenticated actor")))
		return
	}

	result, err := h.projectSvc.SwitchProject(c.Request.Context(), actor, projectID)
	if err != nil {
		c.Error(err)
		return
	}

	h.setRefreshCookie(c, result.RefreshToken)
	c.JSON(http.StatusOK, dto.SwitchOrgResponse{
		AccessToken: result.AccessToken, TokenType: "Bearer", ExpiresIn: result.ExpiresIn,
	})
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
