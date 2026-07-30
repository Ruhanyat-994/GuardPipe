package handler

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/gin-gonic/gin"
)

// Pinger is the one thing Readyz needs from the store — a narrow interface
// so this handler doesn't depend on the whole repo.Store type.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler implements the system endpoints in
// documentation/07-api-specification.md §9. These sit outside /api/v1 and
// are never versioned with it.
type HealthHandler struct {
	db        Pinger
	version   string
	commitSHA string
	buildTime string
}

func NewHealthHandler(db Pinger, version, commitSHA, buildTime string) *HealthHandler {
	return &HealthHandler{db: db, version: version, commitSHA: commitSHA, buildTime: buildTime}
}

// Healthz is liveness only — always 200 if the process can respond at all.
func (h *HealthHandler) Healthz(c *gin.Context) {
	c.Status(http.StatusOK)
}

// Readyz checks Postgres reachability (documentation/04-backend-architecture.md
// §10). Redis reachability isn't checked yet — no Redis client exists in
// the codebase until the queue adapter lands in Phase 6; see
// PROGRESS-LOG.md's Phase 2 section for this deviation.
func (h *HealthHandler) Readyz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	c.Status(http.StatusOK)
}

// Version matches documentation/07-api-specification.md §9's shape exactly:
// {version, commit, built_at, go_version}.
func (h *HealthHandler) Version(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version":    h.version,
		"commit":     h.commitSHA,
		"built_at":   h.buildTime,
		"go_version": runtime.Version(),
	})
}
