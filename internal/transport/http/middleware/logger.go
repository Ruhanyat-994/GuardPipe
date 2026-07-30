package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// StructuredLogger logs one line per request: method, path, status,
// duration, request ID, and user ID when authenticated
// (documentation/04-backend-architecture.md §9). It never logs bodies.
//
// Registered after RequestID (needs the ID) but before ErrorMapper, so by
// the time this middleware's post-c.Next() logic runs, ErrorMapper has
// already written the final response and status code.
func StructuredLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		attrs := []any{
			"request_id", RequestIDFromContext(c),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(start).Milliseconds(),
		}
		if actor, ok := ActorFromContext(c); ok {
			attrs = append(attrs, "user_id", actor.UserID.String())
		}
		logger.Info("http request", attrs...)
	}
}
