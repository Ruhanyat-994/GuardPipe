package dto

import (
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/organization"
)

// --- organization identity — BUILD_GUIDE.md Phase 15 ---
// Not yet part of documentation/07-api-specification.md (new
// `/organizations/*`, `/invites/*`, `/auth/switch-org` routes) — this phase
// touches all three two-approval contract docs per CLAUDE.md, and these
// DTOs are the proposed shape for that review, same posture dto/admin.go's
// own doc-debt note already established for Phase 14.

// MemberSummaryResponse matches one row of `GET /organizations/{id}/members`.
type MemberSummaryResponse struct {
	UserID      string     `json:"user_id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	Role        string     `json:"role"`
	IsHome      bool       `json:"is_home"`
	JoinedAt    *time.Time `json:"joined_at,omitempty"`
}

func FromMemberSummary(m organization.MemberSummary) MemberSummaryResponse {
	resp := MemberSummaryResponse{
		UserID: m.UserID.String(), Email: m.Email, DisplayName: m.DisplayName,
		Role: string(m.Role), IsHome: m.IsHome,
	}
	if !m.IsHome {
		joined := m.JoinedAt
		resp.JoinedAt = &joined
	}
	return resp
}

// MemberListResponse matches `GET /organizations/{id}/members`.
type MemberListResponse struct {
	Data []MemberSummaryResponse `json:"data"`
}

// UpdateMemberRoleRequest matches `PATCH /organizations/{id}/members/{userId}`.
type UpdateMemberRoleRequest struct {
	Role string `json:"role" validate:"required,oneof=admin member viewer"`
}

// CreateInviteRequest matches `POST /organizations/{id}/invites`.
type CreateInviteRequest struct {
	Email string `json:"email" validate:"required,email"`
	Role  string `json:"role" validate:"required,oneof=admin member viewer"`
}

// InviteResponse matches `GET /organizations/{id}/invites` and the response
// of `POST /organizations/{id}/invites` (with Token populated exactly once
// — see CreatedInviteResponse below).
type InviteResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

func FromInvite(inv organization.Invite) InviteResponse {
	return InviteResponse{
		ID: inv.ID.String(), Email: inv.Email, Role: string(inv.Role),
		Status: string(inv.Status), ExpiresAt: inv.ExpiresAt, CreatedAt: inv.CreatedAt,
	}
}

// InviteListResponse matches `GET /organizations/{id}/invites`.
type InviteListResponse struct {
	Data []InviteResponse `json:"data"`
}

// CreatedInviteResponse matches `POST /organizations/{id}/invites`'s
// response — Token is the raw invite token, shown exactly this once (never
// retrievable again, the same "shown once" convention project.CredentialInfo's
// masked hint establishes elsewhere, just the opposite direction: here the
// full value is what's shown, precisely because nothing else can ever
// recover it afterward).
type CreatedInviteResponse struct {
	InviteResponse
	Token string `json:"token"`
}

func FromCreatedInvite(c organization.CreatedInvite) CreatedInviteResponse {
	return CreatedInviteResponse{InviteResponse: FromInvite(c.Invite), Token: c.RawToken}
}

// SwitchOrgResponse matches `POST /auth/switch-org` — same shape as
// RefreshResponse (the new refresh token is set as the `gp_refresh` cookie,
// exactly like Login/Refresh already do, never returned in the body; the
// SPA re-fetches `GET /auth/me` for the new org context).
type SwitchOrgResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// SwitchOrgRequest matches `POST /auth/switch-org`.
type SwitchOrgRequest struct {
	OrgID string `json:"org_id" validate:"required"`
}

// MemberOrgResponse is one entry in the org-switcher list
// (`GET /organizations`).
type MemberOrgResponse struct {
	OrgID  string `json:"org_id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	IsHome bool   `json:"is_home"`
}

func FromMemberOrgSummary(m organization.MemberOrgSummary) MemberOrgResponse {
	return MemberOrgResponse{OrgID: m.OrgID.String(), Name: m.Name, Role: string(m.Role), IsHome: m.IsHome}
}

// MemberOrgListResponse matches `GET /organizations`.
type MemberOrgListResponse struct {
	Data []MemberOrgResponse `json:"data"`
}

// PendingInviteResponse matches one row of `GET /invites/mine` — the live,
// in-app invite-notification feed (2026-09-02 follow-up to Phase 15).
type PendingInviteResponse struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	OrgName   string    `json:"org_name"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

func FromPendingInvite(p organization.PendingInvite) PendingInviteResponse {
	return PendingInviteResponse{
		ID: p.ID.String(), OrgID: p.OrgID.String(), OrgName: p.OrgName,
		Role: string(p.Role), ExpiresAt: p.ExpiresAt, CreatedAt: p.CreatedAt,
	}
}

// PendingInviteListResponse matches `GET /invites/mine`.
type PendingInviteListResponse struct {
	Data []PendingInviteResponse `json:"data"`
}
