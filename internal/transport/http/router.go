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
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/billing"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/livescan"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/notification"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/orchestrator"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/organization"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/pentest"
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
	// OrgSvc is BUILD_GUIDE.md Phase 15's multi-member org identity slice —
	// never nil in production (cmd/guardpipe/main.go always wires it).
	OrgSvc organization.Service
	// PentestSvc is Pentest v2's own read-only service (correlated findings,
	// attack surface, evidence, reports, authorization) — never nil in
	// production (cmd/guardpipe/main.go always wires it).
	PentestSvc *pentest.Service
	// LiveScanSvc is BUILD_GUIDE.md Phase 17 Part B's GitHub webhook live
	// scanning. nil (a test router that doesn't need it) leaves its routes
	// unregistered.
	LiveScanSvc livescan.Service
	// NotificationSvc is the scan-completion feed and each user's report-
	// email settings. nil (a test router that doesn't need it) leaves its
	// routes unregistered.
	NotificationSvc *notification.Service
	// BillingSvc is token billing (TOKENIZATION-ARCHITECTURE.md). nil
	// (GUARDPIPE_BILLING_MODE=off, or a test router) leaves /billing/*
	// unregistered. ScanPreviewer backs the cost estimate.
	BillingSvc    *billing.Service
	ScanPreviewer orchestrator.ScanPreviewer
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
	authH := handler.NewAuthHandler(cfg.IdentitySvc, cfg.AdminSvc, cfg.OrgSvc, cfg.ProjectSvc, v, cfg.SecureCookies, cfg.RefreshTokenTTL)
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
		// switch-org (BUILD_GUIDE.md Phase 15) — re-issues a token pair for a
		// different org the caller already holds a membership in.
		auth.POST("/switch-org", requireAuth, requireNotSuspended, authH.SwitchOrg)
		// switch-project (project-collaborators follow-up) — re-issues a
		// token pair scoped to exactly one project the caller holds an
		// accepted collaborator grant on, not the whole of its org.
		auth.POST("/switch-project/:id", requireAuth, requireNotSuspended, authH.SwitchProject)
	}

	// organizations (BUILD_GUIDE.md Phase 15) — membership, invites, and the
	// org-switcher list. adminOnly here means "admin of the org you're
	// currently acting as" (organization.Service re-checks actor.OrgID
	// matches the :id path param — the standing "404, not 403, for
	// cross-org" rule, requireOwnOrg's own doc comment), never platform-
	// operator status (that's requireOperator, a completely different axis).
	orgH := handler.NewOrganizationHandler(cfg.OrgSvc, v)
	organizations := api.Group("/organizations", requireAuth, requireNotSuspended)
	{
		organizations.GET("", middleware.RBAC(viewerAndAbove...), orgH.ListMine)
		organizations.GET("/:id/members", middleware.RBAC(viewerAndAbove...), orgH.ListMembers)
		organizations.PATCH("/:id/members/:userId", middleware.RBAC(adminOnly...), orgH.UpdateMemberRole)
		organizations.DELETE("/:id/members/:userId", middleware.RBAC(adminOnly...), orgH.RemoveMember)
		organizations.POST("/:id/invites", middleware.RBAC(adminOnly...), orgH.CreateInvite)
		organizations.GET("/:id/invites", middleware.RBAC(adminOnly...), orgH.ListInvites)
		organizations.DELETE("/:id/invites/:inviteId", middleware.RBAC(adminOnly...), orgH.RevokeInvite)
	}
	invites := api.Group("/invites", requireAuth, requireNotSuspended)
	{
		// mine (2026-09-02 follow-up to Phase 15) — the live in-app
		// notification feed NotificationPanel.tsx polls; a static sibling of
		// the :id wildcard routes below, which gin's router resolves
		// correctly (an explicit static segment always wins over a wildcard
		// at the same position).
		invites.GET("/mine", orgH.ListMyInvites)
		// AcceptInvite/DeclineInvite are deliberately not RBAC-gated by the
		// invited org's role — the caller isn't a member of that org yet,
		// that's the whole point of these endpoints;
		// organization.Service.AcceptInvite/DeclineInvite are themselves the
		// authorization check (a matching account email, plus either the
		// invite's id — the live-notification flow — or its raw token — the
		// copy-a-link fallback for someone not logged in yet).
		invites.POST("/:id/accept", orgH.AcceptInvite)
		invites.POST("/:id/decline", orgH.DeclineInvite)
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
		// project assignments / scan schedules (BUILD_GUIDE.md Phase 15)
		projects.POST("/:id/assignments", middleware.RBAC(memberAndAbove...), projectH.AssignProject)
		projects.DELETE("/:id/assignments/:userId", middleware.RBAC(memberAndAbove...), projectH.UnassignProject)
		projects.GET("/:id/assignments", middleware.RBAC(viewerAndAbove...), projectH.ListAssignments)
		// project collaborators (project-collaborators follow-up) — unlike
		// assignments above, these actually grant access (see
		// ProjectCollaborator's own doc comment), to a GuardPipe user who
		// need not already be a member of this project's org. Reachable by
		// a project-scoped collaborator session too, but only for the one
		// project it's scoped to — getOwnedProject enforces that centrally.
		projects.POST("/:id/collaborators/invites", middleware.RBAC(memberAndAbove...), projectH.InviteCollaborator)
		projects.GET("/:id/collaborators/invites", middleware.RBAC(viewerAndAbove...), projectH.ListCollaboratorInvites)
		projects.DELETE("/:id/collaborators/invites/:inviteId", middleware.RBAC(memberAndAbove...), projectH.RevokeCollaboratorInvite)
		projects.GET("/:id/collaborators", middleware.RBAC(viewerAndAbove...), projectH.ListCollaborators)
		projects.DELETE("/:id/collaborators/:userId", middleware.RBAC(memberAndAbove...), projectH.RemoveCollaborator)
	}
	// project-invites (project-collaborators follow-up) — the invitee's own
	// live notification feed and accept/decline, mirroring the /invites
	// group above exactly but for project_invites; kept as its own path so
	// NotificationPanel.tsx can poll both without either handler needing to
	// know about the other kind of invite.
	projectInvites := api.Group("/project-invites", requireAuth, requireNotSuspended)
	{
		projectInvites.GET("/mine", projectH.ListMyProjectInvites)
		projectInvites.POST("/:id/accept", projectH.AcceptCollaboratorInvite)
		projectInvites.POST("/:id/decline", projectH.DeclineCollaboratorInvite)
	}
	// collaborations/mine (project-collaborators follow-up) — every project
	// (any org) the caller holds an accepted collaborator grant on, the
	// "shared projects" switcher's own read.
	api.GET("/collaborations/mine", requireAuth, requireNotSuspended, projectH.ListMyCollaborations)
	// team (BUILD_GUIDE.md Phase 15) — the Team Dashboard's own org-wide
	// assignment read, client-composed with GET /organizations/{id}/members
	// and each project's latest scan the same way Phase 13's
	// GlobalDashboardPage already composes existing endpoints rather than
	// a dedicated aggregate one.
	api.GET("/team/assignments", requireAuth, requireNotSuspended, middleware.RBAC(viewerAndAbove...), projectH.ListAssignmentsForOrg)

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
	// scan schedules (BUILD_GUIDE.md Phase 15)
	projects.POST("/:id/schedules", middleware.RBAC(memberAndAbove...), scanH.CreateSchedule)
	projects.GET("/:id/schedules", middleware.RBAC(viewerAndAbove...), scanH.ListSchedules)
	schedules := api.Group("/schedules", requireAuth, requireNotSuspended)
	{
		schedules.GET("/:id", middleware.RBAC(viewerAndAbove...), scanH.GetSchedule)
		schedules.PATCH("/:id", middleware.RBAC(memberAndAbove...), scanH.UpdateSchedule)
		schedules.DELETE("/:id", middleware.RBAC(memberAndAbove...), scanH.DeleteSchedule)
	}
	scans := api.Group("/scans", requireAuth, requireNotSuspended)
	{
		scans.GET("", middleware.RBAC(viewerAndAbove...), scanH.ListForOrg)
		scans.GET("/active", middleware.RBAC(viewerAndAbove...), scanH.ListActive)
		scans.GET("/:id", middleware.RBAC(viewerAndAbove...), scanH.Get)
		scans.GET("/:id/progress", middleware.RBAC(viewerAndAbove...), scanH.Progress)
		scans.POST("/:id/cancel", middleware.RBAC(memberAndAbove...), scanH.Cancel)
		scans.GET("/:id/findings", middleware.RBAC(viewerAndAbove...), scanH.ListFindings)
		scans.GET("/:id/export", middleware.RBAC(viewerAndAbove...), scanH.Export)
	}

	// GitHub webhook live scanning (BUILD_GUIDE.md Phase 17 Part B).
	// Changing it is adminOnly: it registers a hook on the customer's GitHub
	// repository and makes scans run under the confirming user's name.
	//
	// /webhooks/github/:id is the one route deliberately outside the
	// JWT/RBAC chain — GitHub calls it, not a user. It's authenticated by
	// the X-Hub-Signature-256 HMAC against that webhook's own secret
	// (documentation/12-security-and-threat-model.md S4). The :id (an
	// unguessable UUID) picks which secret to verify with.
	if cfg.LiveScanSvc != nil {
		liveScanH := handler.NewLiveScanHandler(cfg.LiveScanSvc, cfg.Users, v)
		projects.GET("/:id/live-scanning", middleware.RBAC(viewerAndAbove...), liveScanH.Get)
		projects.PUT("/:id/live-scanning", middleware.RBAC(adminOnly...), liveScanH.Enable)
		projects.DELETE("/:id/live-scanning", middleware.RBAC(adminOnly...), liveScanH.Disable)
		api.POST("/webhooks/github/:id", liveScanH.Receive)
	}

	// Scan-completion notifications: the bell's feed, and each user's own
	// report-email settings. /notification-settings/verify is public (the
	// emailed token is the credential) and rate-limited like login, as are
	// the two routes that send an email.
	if cfg.NotificationSvc != nil {
		notifH := handler.NewNotificationHandler(cfg.NotificationSvc, v)
		api.POST("/notification-settings/verify", authLimiter, notifH.VerifyReportEmail)
		mySettings := api.Group("/me/notification-settings", requireAuth, requireNotSuspended)
		{
			mySettings.GET("", notifH.GetSettings)
			mySettings.PUT("", authLimiter, notifH.UpdateSettings)
			mySettings.POST("/resend-verification", authLimiter, notifH.ResendVerification)
			mySettings.POST("/test", authLimiter, notifH.SendTest)
		}
		notifications := api.Group("/notifications", requireAuth, requireNotSuspended)
		{
			notifications.GET("", notifH.List)
			notifications.POST("/read-all", notifH.MarkAllRead)
			notifications.POST("/:id/read", notifH.MarkRead)
		}
	}

	// Token billing. The catalog is public (the pricing page shows it to
	// logged-out visitors); buying and cancelling are admin-only; the
	// demo-checkout confirm answers 404 unless GUARDPIPE_BILLING_MODE=demo.
	if cfg.BillingSvc != nil {
		billingH := handler.NewBillingHandler(cfg.BillingSvc, cfg.ScanPreviewer, cfg.AdminSvc, v)
		api.GET("/billing/catalog", billingH.Catalog)
		billingGroup := api.Group("/billing", requireAuth, requireNotSuspended)
		{
			billingGroup.GET("/summary", middleware.RBAC(viewerAndAbove...), billingH.Summary)
			billingGroup.GET("/ledger", middleware.RBAC(viewerAndAbove...), billingH.Ledger)
			billingGroup.GET("/scans/:id", middleware.RBAC(viewerAndAbove...), billingH.ScanTokens)
			billingGroup.POST("/estimate", middleware.RBAC(memberAndAbove...), billingH.Estimate)
			billingGroup.POST("/checkout", middleware.RBAC(adminOnly...), billingH.StartCheckout)
			billingGroup.GET("/checkout/:id", middleware.RBAC(adminOnly...), billingH.GetCheckout)
			billingGroup.POST("/checkout/:id/confirm", middleware.RBAC(adminOnly...), billingH.ConfirmCheckout)
			billingGroup.POST("/subscription/cancel", middleware.RBAC(adminOnly...), billingH.CancelSubscription)
			billingGroup.POST("/subscription/resume", middleware.RBAC(adminOnly...), billingH.ResumeSubscription)
		}
	}

	// Pentest v2's own read-only API surface (architecture dossier §08) —
	// the deduplicated/correlated findings, attack-surface inventory,
	// evidence, generated-report metadata, and authorization record a
	// pentest scan produces, layered on top of the generic scan/findings
	// endpoints above (which every engine, pentest included, still also
	// populates). Read-only: viewer role is enough for all of it, same as
	// the generic scan endpoints.
	pentestH := handler.NewPentestHandler(cfg.OrchestratorSvc, cfg.PentestSvc)
	pentestScans := api.Group("/pentest/scans", requireAuth, requireNotSuspended, middleware.RBAC(viewerAndAbove...))
	{
		pentestScans.GET("/:id", pentestH.GetScan)
		pentestScans.GET("/:id/attack-surface", pentestH.GetAttackSurface)
		pentestScans.GET("/:id/findings", pentestH.ListFindings)
		pentestScans.GET("/:id/findings/:findingId", pentestH.GetFinding)
		pentestScans.GET("/:id/findings/:findingId/evidence", pentestH.ListEvidenceForFinding)
		pentestScans.GET("/:id/findings/:findingId/evidence/:evidenceId/download", pentestH.DownloadEvidence)
		pentestScans.GET("/:id/reports", pentestH.ListReports)
		pentestScans.GET("/:id/reports/:type", pentestH.GetReport)
		pentestScans.GET("/:id/reports/:type/download", pentestH.DownloadReport)
		pentestScans.GET("/:id/authorization", pentestH.GetAuthorization)
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
		if cfg.BillingSvc != nil {
			billingAdminH := handler.NewBillingHandler(cfg.BillingSvc, cfg.ScanPreviewer, cfg.AdminSvc, v)
			adminGroup.GET("/billing/orgs/:id", requireOperator, billingAdminH.AdminGet)
			adminGroup.POST("/billing/orgs/:id/adjust", requireOperator, billingAdminH.AdminAdjust)
			adminGroup.POST("/billing/orgs/:id/advance-cycle", requireOperator, billingAdminH.AdminAdvanceCycle)
		}
	}

	return r
}
