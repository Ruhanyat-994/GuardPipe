package dto

import (
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/admin"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
)

// --- platform admin panel — BUILD_GUIDE.md Phase 14 ---
// Not yet part of documentation/07-api-specification.md — needs its normal
// two-approval contract-doc addendum before this is "really" done, same
// doc-debt pattern Phase 11/12 already followed (see the phase's own note
// in BUILD_GUIDE.md). These DTOs are the proposed shape for that review.

// OrganizationSummaryResponse matches one row of `GET /admin/organizations`.
type OrganizationSummaryResponse struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	MemberCount     int        `json:"member_count"`
	ProjectCount    int        `json:"project_count"`
	ScanCount       int        `json:"scan_count"`
	SuspendedAt     *time.Time `json:"suspended_at"`
	SuspendedReason *string    `json:"suspended_reason"`
	CreatedAt       time.Time  `json:"created_at"`
}

func FromOrganizationSummary(o admin.OrganizationSummary) OrganizationSummaryResponse {
	return OrganizationSummaryResponse{
		ID: o.ID.String(), Name: o.Name,
		MemberCount: o.MemberCount, ProjectCount: o.ProjectCount, ScanCount: o.ScanCount,
		SuspendedAt: o.SuspendedAt, SuspendedReason: o.SuspendedReason, CreatedAt: o.CreatedAt,
	}
}

// OrganizationListResponse matches `GET /admin/organizations`.
type OrganizationListResponse struct {
	Data       []OrganizationSummaryResponse `json:"data"`
	Pagination Pagination                    `json:"pagination"`
}

// UserSummaryResponse matches an OrganizationDetailResponse's Members entry.
type UserSummaryResponse struct {
	ID              string     `json:"id"`
	OrgID           string     `json:"org_id"`
	Email           string     `json:"email"`
	DisplayName     string     `json:"display_name"`
	Role            string     `json:"role"`
	SuspendedAt     *time.Time `json:"suspended_at"`
	SuspendedReason *string    `json:"suspended_reason"`
	LastLoginAt     *time.Time `json:"last_login_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

func FromUserSummary(u admin.UserSummary) UserSummaryResponse {
	return UserSummaryResponse{
		ID: u.ID.String(), OrgID: u.OrgID.String(), Email: u.Email, DisplayName: u.DisplayName,
		Role: string(u.Role), SuspendedAt: u.SuspendedAt, SuspendedReason: u.SuspendedReason,
		LastLoginAt: u.LastLoginAt, CreatedAt: u.CreatedAt,
	}
}

// OrganizationDetailResponse matches `GET /admin/organizations/{id}`.
type OrganizationDetailResponse struct {
	OrganizationSummaryResponse
	Members []UserSummaryResponse `json:"members"`
}

func FromOrganizationDetail(d admin.OrganizationDetail) OrganizationDetailResponse {
	members := make([]UserSummaryResponse, len(d.Members))
	for i, m := range d.Members {
		members[i] = FromUserSummary(m)
	}
	return OrganizationDetailResponse{OrganizationSummaryResponse: FromOrganizationSummary(d.OrganizationSummary), Members: members}
}

// SuspendRequest matches both `POST /admin/organizations/{id}/suspend` and
// `POST /admin/users/{id}/suspend` — Reason is required (validated in
// modules/admin.Service, not just here) because it's what lands in
// audit_log, the whole point of requiring one.
type SuspendRequest struct {
	Reason string `json:"reason" validate:"required"`
}

// --- pentest misuse flags ---

// CreateFlagRequest matches `POST /admin/pentest-flags`.
type CreateFlagRequest struct {
	TargetID string `json:"target_id" validate:"required"`
	Source   string `json:"source" validate:"required"`
	Reason   string `json:"reason" validate:"required"`
}

// ResolveFlagRequest matches `PATCH /admin/pentest-flags/{id}`.
type ResolveFlagRequest struct {
	Status string `json:"status" validate:"required"`
}

// TargetInfoResponse is the flag-detail's target context.
type TargetInfoResponse struct {
	TargetID    string `json:"target_id"`
	Host        string `json:"host"`
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	OrgID       string `json:"org_id"`
	OrgName     string `json:"org_name"`
}

// PentestFlagResponse matches one row of `GET /admin/pentest-flags` and the
// response of `POST`/`PATCH` against it.
type PentestFlagResponse struct {
	ID         string     `json:"id"`
	Status     string     `json:"status"`
	Source     string     `json:"source"`
	Reason     string     `json:"reason"`
	ReportedBy *string    `json:"reported_by"`
	ResolvedBy *string    `json:"resolved_by"`
	ResolvedAt *time.Time `json:"resolved_at"`
	CreatedAt  time.Time  `json:"created_at"`
	Target     TargetInfoResponse `json:"target"`
}

func FromPentestFlagDetail(f admin.PentestFlagDetail) PentestFlagResponse {
	resp := PentestFlagResponse{
		ID: f.ID.String(), Status: string(f.Status), Source: string(f.Source), Reason: f.Reason,
		ResolvedAt: f.ResolvedAt, CreatedAt: f.CreatedAt,
		Target: TargetInfoResponse{
			TargetID: f.Target.TargetID.String(), Host: f.Target.Host,
			ProjectID: f.Target.ProjectID.String(), ProjectName: f.Target.ProjectName,
			OrgID: f.Target.OrgID.String(), OrgName: f.Target.OrgName,
		},
	}
	if f.ReportedBy != nil {
		s := f.ReportedBy.String()
		resp.ReportedBy = &s
	}
	if f.ResolvedBy != nil {
		s := f.ResolvedBy.String()
		resp.ResolvedBy = &s
	}
	return resp
}

// PentestFlagListResponse matches `GET /admin/pentest-flags`.
type PentestFlagListResponse struct {
	Data       []PentestFlagResponse `json:"data"`
	Pagination Pagination            `json:"pagination"`
}

// --- audit log ---

// AuditEntryResponse matches one row of `GET /admin/audit-log` — the one
// screen in the product where org_id is deliberately shown per-row instead
// of being implied by "your org," since this view spans every organisation.
type AuditEntryResponse struct {
	ID           int64          `json:"id"`
	OrgID        *string        `json:"org_id"`
	ActorID      *string        `json:"actor_id"`
	Action       string         `json:"action"`
	ResourceType *string        `json:"resource_type"`
	ResourceID   *string        `json:"resource_id"`
	Detail       map[string]any `json:"detail"`
	IP           *string        `json:"ip"`
	CreatedAt    time.Time      `json:"created_at"`
}

func FromAuditEntry(e audit.Entry) AuditEntryResponse {
	resp := AuditEntryResponse{
		ID: e.ID, Action: e.Action, ResourceType: e.ResourceType,
		Detail: e.Detail, CreatedAt: e.CreatedAt,
	}
	if e.OrgID != nil {
		s := e.OrgID.String()
		resp.OrgID = &s
	}
	if e.ActorID != nil {
		s := e.ActorID.String()
		resp.ActorID = &s
	}
	if e.ResourceID != nil {
		s := e.ResourceID.String()
		resp.ResourceID = &s
	}
	if e.IP != nil {
		s := e.IP.String()
		resp.IP = &s
	}
	if resp.Detail == nil {
		resp.Detail = map[string]any{}
	}
	return resp
}

// AuditLogListResponse matches `GET /admin/audit-log`.
type AuditLogListResponse struct {
	Data       []AuditEntryResponse `json:"data"`
	Pagination Pagination           `json:"pagination"`
}

// --- system health ---

// EngineJobStatsResponse is one engine's row in SystemHealthResponse.
type EngineJobStatsResponse struct {
	Engine    string `json:"engine"`
	Succeeded int    `json:"succeeded"`
	Failed    int    `json:"failed"`
	Skipped   int    `json:"skipped"`
}

// SystemHealthResponse matches `GET /admin/system-health`. Every field that
// couldn't be read honestly comes back as its zero-ish "unavailable" state
// rather than a fabricated number — SandboxContainersRunning is nil,
// Gemini/AICache carry Available: false — matching this product's existing
// AiPanel unavailable-state convention.
type SystemHealthResponse struct {
	EngineStats              []EngineJobStatsResponse `json:"engine_stats"`
	JobsInFlight              int                      `json:"jobs_in_flight"`
	SandboxContainersRunning *int                      `json:"sandbox_containers_running"`
	Gemini                   GeminiPoolStatusResponse  `json:"gemini"`
	AICache                  AICacheStatusResponse     `json:"ai_cache"`
	CheckedAt                time.Time                 `json:"checked_at"`
}

type GeminiPoolStatusResponse struct {
	Available    bool `json:"available"`
	PoolSize     int  `json:"pool_size"`
	CurrentIndex int  `json:"current_index"`
}

type AICacheStatusResponse struct {
	Available bool `json:"available"`
	Hits      int  `json:"hits"`
	Misses    int  `json:"misses"`
}

func FromSystemHealth(h admin.SystemHealth) SystemHealthResponse {
	stats := make([]EngineJobStatsResponse, len(h.EngineStats))
	for i, s := range h.EngineStats {
		stats[i] = EngineJobStatsResponse{Engine: string(s.Engine), Succeeded: s.Succeeded, Failed: s.Failed, Skipped: s.Skipped}
	}
	return SystemHealthResponse{
		EngineStats:              stats,
		JobsInFlight:              h.JobsInFlight,
		SandboxContainersRunning: h.SandboxContainersRunning,
		Gemini:                   GeminiPoolStatusResponse{Available: h.Gemini.Available, PoolSize: h.Gemini.PoolSize, CurrentIndex: h.Gemini.CurrentIndex},
		AICache:                  AICacheStatusResponse{Available: h.AICache.Available, Hits: h.AICache.Hits, Misses: h.AICache.Misses},
		CheckedAt:                h.CheckedAt,
	}
}
