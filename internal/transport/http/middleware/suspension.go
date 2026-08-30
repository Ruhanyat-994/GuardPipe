package middleware

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// SuspensionCheck rejects a suspended user or a member of a suspended
// organisation with 403 auth.account_suspended, on every authenticated
// request — not just at Login (BUILD_GUIDE.md Phase 14). Must run after
// Auth, which is what populates the Actor this reads. Unlike Auth's plain
// JWT signature check, this needs one indexed database lookup per request
// (identity.Service.CheckSuspension) — the cost of making a suspension
// take effect immediately rather than waiting out the access token's TTL.
func SuspensionCheck(svc identity.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := ActorFromContext(c)
		if !ok {
			// Same programmer-error shape as RBAC's own guard — this
			// middleware is meaningless without Auth having run first.
			c.Error(apperrors.Internal(errors.New("SuspensionCheck middleware used on a route without Auth")))
			c.Abort()
			return
		}
		if err := svc.CheckSuspension(c.Request.Context(), actor); err != nil {
			c.Error(err)
			c.Abort()
			return
		}
		c.Next()
	}
}
