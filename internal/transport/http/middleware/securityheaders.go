package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders sets the headers named in documentation/04-backend-architecture.md
// §4.1. CSP is `default-src 'none'` because this is a JSON API that never
// returns HTML (documentation/07-api-specification.md §11) — there's no
// script/style/image content of its own to allow.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		c.Header("Content-Security-Policy", "default-src 'none'")
		c.Next()
	}
}
