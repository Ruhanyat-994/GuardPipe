package middleware

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// Recovery catches a panic from anywhere downstream, logs it with a stack
// trace, and renders a generic 500 — a panic must never take the process
// down (documentation/04-backend-architecture.md §6.4, NFR-REL-001) and must
// never leak the panic value to the client (the same rule that keeps
// Internal() errors detail-free).
//
// Must run early in the chain — it can only catch panics from middleware
// and handlers registered after it.
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("panic recovered",
					"request_id", RequestIDFromContext(c),
					"method", c.Request.Method,
					"path", c.Request.URL.Path,
					"panic", fmt.Sprint(r),
					"stack", string(debug.Stack()),
				)
				WriteProblem(c, apperrors.Internal(fmt.Errorf("panic: %v", r)))
			}
		}()
		c.Next()
	}
}
