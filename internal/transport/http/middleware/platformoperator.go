package middleware

import (
	"context"
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// OperatorChecker is the narrow slice of admin.Service this middleware
// needs — declared here rather than importing modules/admin's whole
// Service interface, the same "middleware depends on a small interface
// its owning module happens to satisfy" shape Auth(identity.Service)
// already uses, just narrower still.
type OperatorChecker interface {
	IsOperator(ctx context.Context, userID uuid.UUID) (bool, error)
}

// RequirePlatformOperator gates every `/admin/*` route (BUILD_GUIDE.md
// Phase 14) — distinct from RBAC, which checks the existing per-organisation
// domain.Role. Must run after Auth (reads the Actor RBAC/Auth already
// populate) and is checked in *addition* to Auth, never instead of it: an
// operator still needs a valid access token first. The service layer
// (modules/admin) does not re-check operator status itself — see that
// package's own Service doc comment for why that's a deliberate exception
// to the usual "route gates, service re-verifies" split.
func RequirePlatformOperator(svc OperatorChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, ok := ActorFromContext(c)
		if !ok {
			c.Error(apperrors.Internal(errors.New("RequirePlatformOperator middleware used on a route without Auth")))
			c.Abort()
			return
		}
		isOperator, err := svc.IsOperator(c.Request.Context(), actor.UserID)
		if err != nil {
			c.Error(err)
			c.Abort()
			return
		}
		if !isOperator {
			// A plain 403, not 404 — this is a permission boundary on a
			// route that obviously exists (unlike the cross-tenant-resource
			// case documentation/15-testing-strategy.md's "404 not 403"
			// rule actually targets), so there's nothing to hide by
			// pretending /admin/* doesn't exist.
			c.Error(apperrors.Forbidden("admin.operator_required", "this action requires platform operator access"))
			c.Abort()
			return
		}
		c.Next()
	}
}
