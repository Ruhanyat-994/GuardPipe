// Package http assembles the Gin router: the whole API in one file, per
// documentation's package-organisation convention.
package http

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/admin"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/advisory"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/ai"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/reporting"
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

	IdentitySvc     identity.Service
	ProjectSvc      project.Service
	AdvisorySvc     advisory.Service
	OrchestratorSvc orchestrator.Service
	// AdminSvc is BUILD_GUIDE.md Phase 14's platform-operator control plane
	// — never nil in production (cmd/guardpipe/main.go always wires it),
	// kept as its own field rather than folded into an existing service so
	// a test router that doesn't need it can simply omit it.
	AdminSvc admin.Service
	// Users backs the export report's accountability watermark (who
	// requested this scan) — reporting.UserReader, satisfied directly by
	// *store/repo.UserRepo.
	Users reporting.UserReader
	// AISvc may be nil (GUARDPIPE_AI_ENABLED=false or no Gemini key
	// configured, same convention every AI-consuming engine already
	// follows) — the export endpoint's executive summary is then simply
	// omitted, never a reason to fail the whole report (reporting.Assembler's
	// own fallback contract).
	AISvc    ai.Service
	HealthDB handler.Pinger

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
	authH := handler.NewAuthHandler(cfg.IdentitySvc, cfg.AdminSvc, v, cfg.SecureCookies, cfg.RefreshTokenTTL)
	authLimiter := middleware.RateLimit(cfg.AuthRateLimit, cfg.AuthRateWindow)
	requireAuth := middleware.Auth(cfg.IdentitySvc)
	// requireNotSuspended (BUILD_GUIDE.md Phase 14) runs on every
	// authenticated route right after requireAuth — a suspended user or
	// organisation is rejected on the very next request, not just at
	// login. Deliberately not attached to /auth/login itself (Login checks
	// suspension inline, before a token exists to attach an Actor to) or to
	// /auth/refresh (cookie-authenticated, no Auth middleware either).
	requireNotSuspended := middleware.SuspensionCheck(cfg.IdentitySvc)

	api := r.Group("/api/v1")

	docsH := handler.NewDocsHandler()
	api.GET("/openapi.yaml", docsH.OpenAPISpec)

	auth := api.Group("/auth")
	{
		auth.POST("/register", authLimiter, authH.Register)
		auth.POST("/login", authLimiter, authH.Login)
		auth.POST("/refresh", authH.Refresh) // cookie-authenticated, not bearer — no Auth middleware
		auth.POST("/logout", requireAuth, requireNotSuspended, authH.Logout)
		auth.GET("/me", requireAuth, requireNotSuspended, authH.Me)
	}

	projectH := handler.NewProjectHandler(cfg.ProjectSvc, v)
	projects := api.Group("/projects", requireAuth, requireNotSuspended)
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
		projects.GET("/:id/documents", middleware.RBAC(viewerAndAbove...), projectH.ListDocuments)
		projects.POST("/:id/documents", middleware.RBAC(memberAndAbove...), projectH.UploadDocument)
		projects.POST("/:id/documents/import", middleware.RBAC(memberAndAbove...), projectH.ImportDocument)
	}

	targets := api.Group("/targets", requireAuth, requireNotSuspended)
	{
		targets.POST("/:id/attest", middleware.RBAC(memberAndAbove...), projectH.AttestTarget)
		targets.DELETE("/:id", middleware.RBAC(memberAndAbove...), projectH.RevokeTarget)
	}

	documents := api.Group("/documents", requireAuth, requireNotSuspended)
	{
		documents.DELETE("/:id", middleware.RBAC(memberAndAbove...), projectH.DeleteDocument)
	}

	reportsAssembler := reporting.NewAssembler(cfg.OrchestratorSvc, cfg.ProjectSvc, cfg.Users, cfg.AISvc, cfg.Logger)
	scanH := handler.NewScanHandler(cfg.OrchestratorSvc, reportsAssembler, v)
	projects.POST("/:id/scans", middleware.RBAC(memberAndAbove...), scanH.Create)
	projects.GET("/:id/scans", middleware.RBAC(viewerAndAbove...), scanH.List)
	scans := api.Group("/scans", requireAuth, requireNotSuspended)
	{
		scans.GET("", middleware.RBAC(viewerAndAbove...), scanH.ListForOrg)
		scans.GET("/:id", middleware.RBAC(viewerAndAbove...), scanH.Get)
		scans.GET("/:id/progress", middleware.RBAC(viewerAndAbove...), scanH.Progress)
		scans.POST("/:id/cancel", middleware.RBAC(memberAndAbove...), scanH.Cancel)
		scans.GET("/:id/findings", middleware.RBAC(viewerAndAbove...), scanH.ListFindings)
		scans.GET("/:id/export", middleware.RBAC(viewerAndAbove...), scanH.Export)
	}

	// requireOperator (BUILD_GUIDE.md Phase 14) gates every `/admin/*`
	// route, in addition to requireAuth+requireNotSuspended — see
	// middleware.RequirePlatformOperator's own doc comment.
	requireOperator := middleware.RequirePlatformOperator(cfg.AdminSvc)

	ruleH := handler.NewRuleHandler(cfg.AdvisorySvc, v)
	rules := api.Group("/rules", requireAuth, requireNotSuspended)
	{
		rules.GET("", middleware.RBAC(viewerAndAbove...), ruleH.List)
		rules.GET("/:id", middleware.RBAC(viewerAndAbove...), ruleH.Get)
		// Re-gated from org-scoped adminOnly to requireOperator
		// (BUILD_GUIDE.md Phase 14): `rules` (documentation/06-database-design.md
		// §4.15) is a single global catalogue, not org-scoped — as written
		// before this phase, any org's own admin could disable a detection
		// rule for every tenant on the platform, not just their own.
		rules.PATCH("/:id", requireOperator, ruleH.SetEnabled)
	}

	adminH := handler.NewAdminHandler(cfg.AdminSvc, v)
	adminGroup := api.Group("/admin", requireAuth, requireNotSuspended)
	{
		adminGroup.GET("/organizations", requireOperator, adminH.ListOrganizations)
		adminGroup.GET("/organizations/:id", requireOperator, adminH.GetOrganization)
		adminGroup.POST("/organizations/:id/suspend", requireOperator, adminH.SuspendOrganization)
		adminGroup.POST("/organizations/:id/reinstate", requireOperator, adminH.ReinstateOrganization)
		adminGroup.POST("/users/:id/suspend", requireOperator, adminH.SuspendUser)
		adminGroup.POST("/users/:id/reinstate", requireOperator, adminH.ReinstateUser)
		// CreateFlag is deliberately not behind requireOperator — see
		// AdminHandler.CreateFlag's own doc comment.
		adminGroup.POST("/pentest-flags", adminH.CreateFlag)
		adminGroup.GET("/pentest-flags", requireOperator, adminH.ListFlags)
		adminGroup.PATCH("/pentest-flags/:id", requireOperator, adminH.ResolveFlag)
		adminGroup.GET("/audit-log", requireOperator, adminH.ListAuditLog)
		adminGroup.GET("/system-health", requireOperator, adminH.SystemHealth)
	}

	return r
}
