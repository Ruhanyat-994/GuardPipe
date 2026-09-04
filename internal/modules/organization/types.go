// Package organization is BUILD_GUIDE.md Phase 15's "organization identity"
// slice — multi-member orgs (organization_memberships), email invites
// (organization_invites), switch-org, and the last-Owner (NIST INCITS
// 359-2012 Hierarchical RBAC, `admin` standing in for the doc's "Owner"
// tier — domain.Role has no separate fourth tier) constraint. Google OIDC/
// domain-restricted SSO was pulled out of this pass at the user's explicit
// request — organization_domains/organization_sso_settings (migration
// 00020) exist as reserved, unused schema only; nothing in this package
// reads or writes them.
package organization

import (
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
)

// Membership mirrors one row of `organization_memberships`
// (documentation/06-database-design.md — Phase 15 addendum, doc debt noted
// in service.go). It is purely additive to a user's own home organisation
// (users.org_id, identity-owned, left completely unchanged) — see
// CLAUDE.md's "Multi-tenancy" section for why that distinction matters.
type Membership struct {
	ID        uuid.UUID
	OrgID     uuid.UUID
	UserID    uuid.UUID
	Role      domain.Role
	InvitedBy *uuid.UUID
	CreatedAt time.Time
}

// MemberSummary is one row of `GET /organizations/{id}/members` — a
// Membership (or, for the org's own founder, their identity.User row)
// joined with just enough user identity to render a member list. IsHome is
// true exactly when this member's users.org_id already equals the
// organisation (the founder) rather than being here via a
// Membership row — RemoveMember refuses to remove a home member (there is
// no membership row to delete; "leaving" your own home org isn't a
// supported operation, deleting the account would be).
type MemberSummary struct {
	UserID      uuid.UUID
	Email       string
	DisplayName string
	Role        domain.Role
	IsHome      bool
	JoinedAt    time.Time
}

// InviteStatus matches the `invite_status` Postgres enum (migration 00019).
type InviteStatus string

const (
	InvitePending  InviteStatus = "pending"
	InviteAccepted InviteStatus = "accepted"
	InviteExpired  InviteStatus = "expired"
	InviteRevoked  InviteStatus = "revoked"
	// InviteDeclined (migration 00023) is set by the invitee themselves
	// (DeclineInvite) — distinct from InviteRevoked (set by an org admin,
	// RevokeInvite) so ListInvites can honestly show which happened.
	InviteDeclined InviteStatus = "declined"
)

func (s InviteStatus) Valid() bool {
	switch s {
	case InvitePending, InviteAccepted, InviteExpired, InviteRevoked, InviteDeclined:
		return true
	default:
		return false
	}
}

// Invite mirrors one row of `organization_invites`. TokenHash is the only
// form the token is ever persisted in — the same pattern refresh_tokens
// already established (identity.RefreshToken's own doc comment) — the raw
// token is returned exactly once, from CreateInvite's response, and never
// retrievable again.
type Invite struct {
	ID        uuid.UUID
	OrgID     uuid.UUID
	Email     string
	Role      domain.Role
	InvitedBy *uuid.UUID
	TokenHash string
	Status    InviteStatus
	ExpiresAt time.Time
	CreatedAt time.Time
}

// InviteInput is CreateInvite's input.
type InviteInput struct {
	Email string
	Role  domain.Role
}

// PendingInvite is one row of `GET /invites/mine` — the live, in-app
// notification feed for an already-registered account (2026-09-02 follow-up
// to Phase 15: "generate a link and copy it" isn't the primary flow for
// someone who already has an account — they should see a real notification
// they can accept or decline directly, the same way a scan-completion alert
// eventually will, once one exists). OrgName is why this is its own type
// rather than reusing Invite verbatim — a bare org_id is useless to render.
type PendingInvite struct {
	Invite
	OrgName string
}

// CreatedInvite is CreateInvite's return value — the Invite row plus the
// one-time-visible raw token (there is no GUARDPIPE email adapter in this
// build; the operator shares the resulting `/invites/{token}/accept` link
// out of band, the same "shown once" convention project.CredentialInfo's
// masked hint already establishes for GitHub PATs).
type CreatedInvite struct {
	Invite
	RawToken string
}

// SwitchOrgResult is `POST /auth/switch-org`'s response — a fresh,
// independent token pair scoped to the target organisation (see
// identity.Service.IssueTokenPairForOrg's own doc comment on why it's
// always a new rotation family).
type SwitchOrgResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

// MemberOrgSummary is one entry in the org-switcher list (`GET
// /organizations`, the caller's own memberships) — every organisation the
// caller can currently switch into, home org included.
type MemberOrgSummary struct {
	OrgID  uuid.UUID
	Name   string
	Role   domain.Role
	IsHome bool
}
