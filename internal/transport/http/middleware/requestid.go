// Package middleware implements the HTTP middleware chain in
// documentation/04-backend-architecture.md §4.1: RequestID → Recovery →
// StructuredLogger → CORS → SecurityHeaders → RateLimit → Auth → RBAC →
// ErrorMapper → handler.
package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDHeader is the header every response carries
// (documentation/07-api-specification.md, "Correlation").
const RequestIDHeader = "X-Request-ID"

const requestIDContextKey = "request_id"

// RequestID generates (or propagates, if the caller already sent one) a
// request ID and makes it available to every later middleware and handler.
// Must be first in the chain so every subsequent log line can carry it.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := c.GetHeader(RequestIDHeader)
		if reqID == "" {
			reqID = uuid.NewString()
		}
		c.Set(requestIDContextKey, reqID)
		c.Header(RequestIDHeader, reqID)
		c.Next()
	}
}

// RequestIDFromContext reads back what RequestID set, or "" if it wasn't
// run (which should never happen outside of a unit test calling a handler
// directly).
func RequestIDFromContext(c *gin.Context) string {
	if v, ok := c.Get(requestIDContextKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
