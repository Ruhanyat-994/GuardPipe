package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

const actorContextKey = "actor"

// Auth verifies the bearer access token and loads the caller's identity
// into context as a domain.Actor. Public routes opt out by simply not being
// registered under a group that uses this middleware
// (documentation/04-backend-architecture.md §4.1) — there is no per-route
// skip flag to forget.
func Auth(svc identity.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(header, prefix) || strings.TrimPrefix(header, prefix) == "" {
			c.Error(apperrors.Unauthorized("auth.token_invalid", "missing bearer token"))
			c.Abort()
			return
		}
		token := strings.TrimPrefix(header, prefix)

		claims, err := svc.Verify(c.Request.Context(), token)
		if err != nil {
			c.Error(err)
			c.Abort()
			return
		}

		c.Set(actorContextKey, domain.Actor{UserID: claims.UserID, OrgID: claims.OrgID, Role: claims.Role})
		c.Next()
	}
}

// ActorFromContext reads back the Actor Auth set. The bool is false if Auth
// didn't run — callers reaching for this without an Auth-protected route is
// a programmer error, not a request-time condition.
func ActorFromContext(c *gin.Context) (domain.Actor, bool) {
	v, ok := c.Get(actorContextKey)
	if !ok {
		return domain.Actor{}, false
	}
	actor, ok := v.(domain.Actor)
	return actor, ok
}
