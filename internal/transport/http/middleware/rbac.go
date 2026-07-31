package middleware

import (
	"errors"
	"slices"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// RBAC enforces that the authenticated actor's role is one of allowed —
// route-level authorisation only. Resource-level ownership (does this
// user's org own this specific object?) is the service layer's job and
// cannot be skipped just because RBAC passed
// (documentation/04-backend-architecture.md §11, "Authorisation — two
// layers"). Must run after Auth, which is what populates the Actor RBAC
// reads.
func RBAC(allowed ...domain.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := ActorFromContext(c)
		if !ok {
			c.Error(apperrors.Internal(errors.New("RBAC middleware used on a route without Auth")))
			c.Abort()
			return
		}
		if slices.Contains(allowed, actor.Role) {
			c.Next()
			return
		}
		c.Error(apperrors.Forbidden("auth.forbidden_role", "your role does not permit this action"))
		c.Abort()
	}
}
