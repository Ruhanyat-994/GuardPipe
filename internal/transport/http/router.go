// Package http assembles the Gin router: the whole API in one file, per
// documentation's package-organisation convention.
package http

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/handler"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/middleware"
)

// Role tiers for RBAC (documentation/07-api-specification.md §3-4's "Role"
// column): higher roles can do everything a lower one can, so each tier
// lists itself and everything above it.
var (
	viewerAndAbove = []domain.Role{domain.RoleViewer, domain.RoleMember, domain.RoleAdmin}
	memberAndAbove = []domain.Role{domain.RoleMember, domain.RoleAdmin}
	adminOnly      = []domain.Role{domain.RoleAdmin}
)

// RouterConfig is everything NewRouter needs — assembled once in
// cmd/guardpipe/main.go from platform/config and the wired-up modules.
type RouterConfig struct {
	Logger      *slog.Logger
	CORSOrigins []string

	IdentitySvc identity.Service
	ProjectSvc  project.Service
	HealthDB    handler.Pinger

	Version   string
	CommitSHA string
	BuildTime string

	SecureCookies   bool // true in production (HTTPS); false for local dev over HTTP
	RefreshTokenTTL time.Duration

	AuthRateLimit  int
	AuthRateWindow time.Duration
}

// NewRouter builds the whole API. The middleware chain order matches
// documentation/04-backend-architecture.md §4.1 exactly: RequestID →
// Recovery → StructuredLogger → CORS → SecurityHeaders → ErrorMapper are
// global; RateLimit/Auth/RBAC are attached per route, since not every route
// needs them, but they still end up positioned after the global chain and
// before the handler in Gin's execution order.
func NewRouter(cfg RouterConfig) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.Use(
		middleware.RequestID(),
		middleware.Recovery(cfg.Logger),
		middleware.StructuredLogger(cfg.Logger),
		middleware.CORS(cfg.CORSOrigins),
		middleware.SecurityHeaders(),
		middleware.ErrorMapper(),
	)

	healthH := handler.NewHealthHandler(cfg.HealthDB, cfg.Version, cfg.CommitSHA, cfg.BuildTime)
	r.GET("/healthz", healthH.Healthz)
	r.GET("/readyz", healthH.Readyz)
	r.GET("/version", healthH.Version)

	v := validate.New()
	authH := handler.NewAuthHandler(cfg.IdentitySvc, v, cfg.SecureCookies, cfg.RefreshTokenTTL)
	authLimiter := middleware.RateLimit(cfg.AuthRateLimit, cfg.AuthRateWindow)
	requireAuth := middleware.Auth(cfg.IdentitySvc)

	api := r.Group("/api/v1")
	auth := api.Group("/auth")
	{
		auth.POST("/register", authLimiter, authH.Register)
		auth.POST("/login", authLimiter, authH.Login)
		auth.POST("/refresh", authH.Refresh) // cookie-authenticated, not bearer — no Auth middleware
		auth.POST("/logout", requireAuth, authH.Logout)
		auth.GET("/me", requireAuth, authH.Me)
	}

	projectH := handler.NewProjectHandler(cfg.ProjectSvc, v)
	projects := api.Group("/projects", requireAuth)
	{
		projects.GET("", middleware.RBAC(viewerAndAbove...), projectH.List)
		projects.POST("", middleware.RBAC(memberAndAbove...), projectH.Create)
		projects.GET("/:id", middleware.RBAC(viewerAndAbove...), projectH.Get)
		projects.PATCH("/:id", middleware.RBAC(memberAndAbove...), projectH.Update)
		projects.DELETE("/:id", middleware.RBAC(adminOnly...), projectH.Archive)
		projects.POST("/:id/repository", middleware.RBAC(memberAndAbove...), projectH.AttachRepository)
		projects.PUT("/:id/credential", middleware.RBAC(memberAndAbove...), projectH.SetCredential)
		projects.DELETE("/:id/credential", middleware.RBAC(memberAndAbove...), projectH.RemoveCredential)
		projects.GET("/:id/targets", middleware.RBAC(viewerAndAbove...), projectH.ListTargets)
		projects.POST("/:id/targets", middleware.RBAC(memberAndAbove...), projectH.RegisterTarget)
	}

	targets := api.Group("/targets", requireAuth)
	{
		targets.POST("/:id/attest", middleware.RBAC(memberAndAbove...), projectH.AttestTarget)
		targets.DELETE("/:id", middleware.RBAC(memberAndAbove...), projectH.RevokeTarget)
	}

	return r
}
