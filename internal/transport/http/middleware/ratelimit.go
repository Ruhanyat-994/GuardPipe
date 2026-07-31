package middleware

import (
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// limiter is a fixed-window counter per key, in-process memory.
//
// Documentation/04-backend-architecture.md §4.1 specifies a Redis token
// bucket — deliberately not what this is. No Redis client exists in the
// codebase yet (the queue/cache adapter lands in Phase 6), and this project
// runs as a single instance (GUARDPIPE_ROLE=all, no horizontal scaling
// requirement — documentation/01-project-charter.md's constraints), so an
// in-memory limiter is functionally equivalent for now. Swap for a
// Redis-backed one if the process is ever run as more than one replica.
// See PROGRESS-LOG.md's Phase 2 section for this deviation.
type limiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	counters map[string]*windowCounter
}

type windowCounter struct {
	count   int
	resetAt time.Time
}

func newLimiter(limit int, window time.Duration) *limiter {
	return &limiter{limit: limit, window: window, counters: make(map[string]*windowCounter)}
}

func (l *limiter) allow(key string) (allowed bool, retryAfter time.Duration, remaining int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	c, ok := l.counters[key]
	if !ok || now.After(c.resetAt) {
		c = &windowCounter{resetAt: now.Add(l.window)}
		l.counters[key] = c
	}
	if c.count >= l.limit {
		return false, time.Until(c.resetAt), 0
	}
	c.count++
	return true, 0, l.limit - c.count
}

// RateLimit limits requests keyed by client IP to limit per window — e.g.
// 5/min on /auth/login and /auth/register (documentation/07-api-specification.md
// §1.6). Every response carries X-RateLimit-* headers; a limited response
// carries Retry-After too (via the RateLimited error's RetryAfter field,
// read by middleware.WriteProblem).
func RateLimit(limit int, window time.Duration) gin.HandlerFunc {
	l := newLimiter(limit, window)
	return func(c *gin.Context) {
		key := c.ClientIP()
		allowed, retryAfter, remaining := l.allow(key)

		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))

		if !allowed {
			seconds := int(retryAfter.Seconds()) + 1
			c.Error(apperrors.RateLimited("ratelimit.exceeded", "too many requests — try again later", seconds))
			c.Abort()
			return
		}
		c.Next()
	}
}
