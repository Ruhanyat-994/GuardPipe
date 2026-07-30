package middleware

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// ErrorMapper is the only place HTTP error bodies are built
// (documentation/04-backend-architecture.md §4.1). Handlers, and every
// middleware after this one in the chain, report failures with
// `c.Error(err); c.Abort()` — never by writing JSON themselves — so this is
// the single place that has to know the RFC 9457 shape.
//
// It must be registered before any middleware or handler that can call
// c.Error (RateLimit, Auth, RBAC, handlers), so that its post-c.Next() logic
// — which runs after everything nested inside it — sees whatever error they
// set.
func ErrorMapper() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		WriteProblem(c, c.Errors.Last().Err)
	}
}

// WriteProblem renders err as an RFC 9457 problem-details response and
// aborts the chain. It's exported (not just used internally by
// ErrorMapper) because Recovery needs it too: a panic unwinds past
// ErrorMapper's own stack frame entirely, so Recovery cannot rely on
// ErrorMapper's post-c.Next() logic running — it has to render the
// response itself, using the same renderer.
func WriteProblem(c *gin.Context, err error) {
	if c.Writer.Written() {
		return
	}

	var appErr *apperrors.Error
	if errors.As(err, &appErr) && appErr.Kind == apperrors.KindRateLimited && appErr.RetryAfter > 0 {
		c.Header("Retry-After", strconv.Itoa(appErr.RetryAfter))
	}

	pd := apperrors.ToProblemDetails(err, c.Request.URL.Path, RequestIDFromContext(c))
	c.AbortWithStatusJSON(pd.Status, pd)
}
