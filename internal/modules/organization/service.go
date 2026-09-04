package organization

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// inviteTTL is how long an invite link stays acceptable — not specified by
// any requirement doc, a conservative default the same way identity's own
// account-lockout constants are (see identity/service.go's own comment on
// maxFailedLogins).
const inviteTTL = 7 * 24 * time.Hour

// Service is BUILD_GUIDE.md Phase 15's multi-member org identity slice —
// not yet part of documentation/07-api-specification.md (new `/organizations/*`
// routes) or documentation/02-srs.md (new FR-IAM-* requirements); this
// phase touches all three two-approval contract docs per CLAUDE.md, and
// this is the proposed shape for that review, same posture Phase 11's own
// schema change already established.
//
// Every method here re-checks that the caller's active org context
// (actor.OrgID, the JWT claim) matches the orgID path parameter —
// documentation/04-backend-architecture.md §11's "two layers": route-level
// RBAC (adminOnly, wired in transport/http/router.go) only checks the
// caller's role, never which org they're acting as.
type Service interface {
	ListMembers(ctx context.Context, actor domain.Actor, orgID uuid.UUID) ([]MemberSummary, error)
	// UpdateMemberRole and RemoveMember both enforce the last-remaining-admin
	// constraint (NIST INCITS 359-2012 Hierarchical RBAC's separation-of-duties
	// principle, CLAUDE.md/BUILD_GUIDE.md's "last Owner" language — this
	// codebase's domain.Role has no separate Owner tier, so `admin` plays
	// that role) — server-side, not just hidden in the UI.
	UpdateMemberRole(ctx context.Context, actor domain.Actor, orgID, userID uuid.UUID, newRole domain.Role) error
	// RemoveMember only ever deletes an organization_memberships row — the
	// org's own founder (whose membership is implicit, via users.org_id, not
	// a membership row) can never be removed through this method, a stated
	// simplification (see MemberSummary.IsHome's own doc comment), not a
	// silently arbitrary choice: "leaving your own home org" isn't a
	// supported operation, only role changes are (UpdateMemberRole already
	// applies to a home member exactly like any other).
	RemoveMember(ctx context.Context, actor domain.Actor, orgID, userID uuid.UUID) error

	CreateInvite(ctx context.Context, actor domain.Actor, orgID uuid.UUID, in InviteInput) (*CreatedInvite, error)
	ListInvites(ctx context.Context, actor domain.Actor, orgID uuid.UUID) ([]Invite, error)
	RevokeInvite(ctx context.Context, actor domain.Actor, orgID, inviteID uuid.UUID) error
	// AcceptInvite requires an authenticated actor (BUILD_GUIDE.md Phase 15's
	// "if the accepting email has no account yet, routes through registration
	// first, then accepts" is a frontend-orchestrated two-call flow — register
	// or log in, then call this — not a public unauthenticated accept; the
	// invited email must match the caller's own account email exactly).
	// identifier is either the invite's raw token (the copy-a-link fallback,
	// for someone not logged in when the invite arrives) or its UUID (the
	// primary flow, 2026-09-02 follow-up: an already-registered invitee sees
	// this invite as a live notification, ListMyInvites below, and accepts
	// it by ID directly — no token involved, since being authenticated as
	// the matching email is already the actual security check either way).
	AcceptInvite(ctx context.Context, actor domain.Actor, identifier string) (*Membership, error)
	// ListMyInvites is the live notification feed's own read — every
	// still-pending invite addressed to the caller's own account email,
	// across every organisation, resolved via UserReader (not a claim the
	// caller makes) so this can never be used to enumerate someone else's
	// invites.
	ListMyInvites(ctx context.Context, actor domain.Actor) ([]PendingInvite, error)
	// DeclineInvite is the invitee's own counterpart to RevokeInvite (an org
	// admin's action) — same email-match requirement as AcceptInvite, marks
	// the invite InviteDeclined rather than InviteRevoked so the inviting
	// org can tell the two apart.
	DeclineInvite(ctx context.Context, actor domain.Actor, inviteID uuid.UUID) error

	// SwitchOrg is `POST /auth/switch-org` — see
	// identity.Service.IssueTokenPairForOrg's own doc comment for why this
	// always mints a brand-new, independent token pair.
	SwitchOrg(ctx context.Context, actor domain.Actor, targetOrgID uuid.UUID) (*SwitchOrgResult, error)
	// ListMemberOrgs backs the org-switcher UI — every organisation the
	// caller can currently switch into, home org included first.
	ListMemberOrgs(ctx context.Context, actor domain.Actor) ([]MemberOrgSummary, error)
}

// MembershipRepository is defined by this package; implementation lives in
// internal/store/repo.
type MembershipRepository interface {
	Create(ctx context.Context, m *Membership) error
	GetByOrgAndUser(ctx context.Context, orgID, userID uuid.UUID) (*Membership, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]Membership, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]Membership, error)
	UpdateRole(ctx context.Context, orgID, userID uuid.UUID, role domain.Role) error
	Delete(ctx context.Context, orgID, userID uuid.UUID) error
}

// InviteRepository is defined by this package; implementation lives in
// internal/store/repo.
type InviteRepository interface {
	Create(ctx context.Context, in *Invite) error
	GetByTokenHash(ctx context.Context, tokenHash string) (*Invite, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Invite, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]Invite, error)
	// ListByEmail backs ListMyInvites — every invite addressed to email,
	// across every org, regardless of status (the service filters to
	// pending, unexpired ones; kept unfiltered here so this one repository
	// method can't drift out of sync with whatever the service considers
	// "still actionable").
	ListByEmail(ctx context.Context, email string) ([]Invite, error)
	// ExistsPending reports whether orgID already has a live (pending,
	// unexpired) invite outstanding for email — CreateInvite's duplicate
	// guard.
	ExistsPending(ctx context.Context, orgID uuid.UUID, email string) (bool, error)
	SetStatus(ctx context.Context, id uuid.UUID, status InviteStatus) error
}

// UserInfo is the narrow slice of identity.User this package needs — never
// the full type (project.UserDisplayNameLookup's own precedent for why:
// this package has no business reading a password hash or lockout state).
type UserInfo struct {
	ID          uuid.UUID
	OrgID       uuid.UUID
	Email       string
	DisplayName string
	Role        domain.Role
}

// UserReader is defined by this package; implementation lives in
// internal/store/repo, on the same UserRepo type that already implements
// identity.UserRepository and admin.UserRepository against the `users`
// table — one repository struct satisfying a third module's interface, the
// same pattern UserRepo.GetDisplayName already establishes for `project`.
//
// SetHomeRole is the one write this package makes against identity's own
// table — narrowly scoped (a role column, nothing else) and only ever
// called with userID's own OrgID as orgID (UpdateMemberRole's isHome
// branch), the same "define the narrow interface you need, implemented by
// store/repo" sanctioning pattern admin.UserRepository.SetSuspended already
// establishes against this exact table for a different module.
type UserReader interface {
	// GetInfoByID is named distinctly from identity.UserRepository's own
	// GetByID (which returns a full identity.User) — both interfaces are
	// implemented by the same store/repo.UserRepo struct, and Go doesn't
	// allow two methods of the same name with different signatures on one
	// type (admin.UserRepository.GetSummaryByID's own precedent for this).
	GetInfoByID(ctx context.Context, id uuid.UUID) (*UserInfo, error)
	// ListHomeMembers returns every user whose OWN home org is orgID — in
	// this codebase that is always exactly one row (the org's founder;
	// registration never lets two accounts share users.org_id, CLAUDE.md's
	// "Multi-tenancy" section), read generically rather than assumed, so a
	// future change to that invariant doesn't silently break this package.
	ListHomeMembers(ctx context.Context, orgID uuid.UUID) ([]UserInfo, error)
	SetHomeRole(ctx context.Context, userID uuid.UUID, role domain.Role) error
}

// OrganizationReader is defined by this package; implementation lives in
// internal/store/repo, on the same OrganizationRepo type that already
// implements identity.OrganizationRepository and admin.OrganizationRepository.
type OrganizationReader interface {
	GetName(ctx context.Context, orgID uuid.UUID) (string, error)
}

type service struct {
	memberships MembershipRepository
	invites     InviteRepository
	users       UserReader
	orgs        OrganizationReader
	identitySvc identity.Service
	audit       audit.Service
}

// NewService wires the organization module.
func NewService(
	memberships MembershipRepository,
	invites InviteRepository,
	users UserReader,
	orgs OrganizationReader,
	identitySvc identity.Service,
	auditSvc audit.Service,
) Service {
	return &service{
		memberships: memberships, invites: invites, users: users, orgs: orgs,
		identitySvc: identitySvc, audit: auditSvc,
	}
}

func (s *service) ListMembers(ctx context.Context, actor domain.Actor, orgID uuid.UUID) ([]MemberSummary, error) {
	if err := requireOwnOrg(actor, orgID); err != nil {
		return nil, err
	}

	home, err := s.users.ListHomeMembers(ctx, orgID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list home members: %w", err))
	}
	memberships, err := s.memberships.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list memberships: %w", err))
	}

	out := make([]MemberSummary, 0, len(home)+len(memberships))
	for _, u := range home {
		out = append(out, MemberSummary{UserID: u.ID, Email: u.Email, DisplayName: u.DisplayName, Role: u.Role, IsHome: true})
	}
	for _, m := range memberships {
		u, err := s.users.GetInfoByID(ctx, m.UserID)
		if err != nil {
			return nil, apperrors.Internal(fmt.Errorf("get member user: %w", err))
		}
		out = append(out, MemberSummary{
			UserID: m.UserID, Email: u.Email, DisplayName: u.DisplayName,
			Role: m.Role, IsHome: false, JoinedAt: m.CreatedAt,
		})
	}
	return out, nil
}

func (s *service) UpdateMemberRole(ctx context.Context, actor domain.Actor, orgID, userID uuid.UUID, newRole domain.Role) error {
	if err := requireOwnOrg(actor, orgID); err != nil {
		return err
	}
	if !newRole.Valid() {
		return apperrors.Validation("organization.invalid_input", "role must be a recognised role", nil)
	}

	isHome, currentRole, err := s.resolveMember(ctx, orgID, userID)
	if err != nil {
		return err
	}
	if currentRole == newRole {
		return nil // idempotent no-op
	}
	if currentRole == domain.RoleAdmin && newRole != domain.RoleAdmin {
		if err := s.requireNotLastAdmin(ctx, orgID); err != nil {
			return err
		}
	}

	if isHome {
		if err := s.users.SetHomeRole(ctx, userID, newRole); err != nil {
			return apperrors.Internal(fmt.Errorf("set home member role: %w", err))
		}
	} else if err := s.memberships.UpdateRole(ctx, orgID, userID, newRole); err != nil {
		return apperrors.Internal(fmt.Errorf("update membership role: %w", err))
	}

	s.audit.Log(ctx, audit.Entry{
		OrgID: &orgID, ActorID: &actor.UserID, Action: "organization.member_role_updated",
		ResourceType: strPtr("user"), ResourceID: &userID,
		Detail: map[string]any{"new_role": string(newRole)},
	})
	return nil
}

func (s *service) RemoveMember(ctx context.Context, actor domain.Actor, orgID, userID uuid.UUID) error {
	if err := requireOwnOrg(actor, orgID); err != nil {
		return err
	}
	isHome, currentRole, err := s.resolveMember(ctx, orgID, userID)
	if err != nil {
		return err
	}
	if isHome {
		return apperrors.Validation("organization.cannot_remove_founder", "the organization's founder cannot be removed", nil)
	}
	if currentRole == domain.RoleAdmin {
		if err := s.requireNotLastAdmin(ctx, orgID); err != nil {
			return err
		}
	}
	if err := s.memberships.Delete(ctx, orgID, userID); err != nil {
		return apperrors.Internal(fmt.Errorf("remove member: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &orgID, ActorID: &actor.UserID, Action: "organization.member_removed",
		ResourceType: strPtr("user"), ResourceID: &userID,
	})
	return nil
}

func (s *service) CreateInvite(ctx context.Context, actor domain.Actor, orgID uuid.UUID, in InviteInput) (*CreatedInvite, error) {
	if err := requireOwnOrg(actor, orgID); err != nil {
		return nil, err
	}
	email := normalizeEmail(in.Email)
	if email == "" {
		return nil, apperrors.Validation("organization.invalid_input", "email is required", nil)
	}
	if !in.Role.Valid() {
		return nil, apperrors.Validation("organization.invalid_input", "role must be a recognised role", nil)
	}

	if already, err := s.isAlreadyMember(ctx, orgID, email); err != nil {
		return nil, err
	} else if already {
		return nil, apperrors.Conflict("organization.already_member", "this email already belongs to the organization")
	}
	if pending, err := s.invites.ExistsPending(ctx, orgID, email); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("check pending invite: %w", err))
	} else if pending {
		return nil, apperrors.Conflict("organization.invite_already_pending", "an invite is already pending for this email")
	}

	rawToken, tokenHash, err := newInviteToken()
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("generate invite token: %w", err))
	}
	invitedBy := actor.UserID
	inv := &Invite{
		ID: id.New(), OrgID: orgID, Email: email, Role: in.Role, InvitedBy: &invitedBy,
		TokenHash: tokenHash, Status: InvitePending, ExpiresAt: time.Now().UTC().Add(inviteTTL),
	}
	if err := s.invites.Create(ctx, inv); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("create invite: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &orgID, ActorID: &actor.UserID, Action: "organization.invite_created",
		ResourceType: strPtr("organization_invite"), ResourceID: &inv.ID,
		Detail: map[string]any{"email": email, "role": string(in.Role)},
	})
	return &CreatedInvite{Invite: *inv, RawToken: rawToken}, nil
}

func (s *service) ListInvites(ctx context.Context, actor domain.Actor, orgID uuid.UUID) ([]Invite, error) {
	if err := requireOwnOrg(actor, orgID); err != nil {
		return nil, err
	}
	invites, err := s.invites.ListByOrg(ctx, orgID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list invites: %w", err))
	}
	return invites, nil
}

func (s *service) RevokeInvite(ctx context.Context, actor domain.Actor, orgID, inviteID uuid.UUID) error {
	if err := requireOwnOrg(actor, orgID); err != nil {
		return err
	}
	inv, err := s.invites.GetByID(ctx, inviteID)
	if err != nil {
		if isNotFound(err) {
			return apperrors.NotFound("organization.invite_not_found", "invite not found")
		}
		return apperrors.Internal(fmt.Errorf("get invite: %w", err))
	}
	if inv.OrgID != orgID {
		return apperrors.NotFound("organization.invite_not_found", "invite not found")
	}
	if inv.Status != InvitePending {
		return apperrors.Unprocessable("organization.invite_not_pending", "only a pending invite can be revoked")
	}
	if err := s.invites.SetStatus(ctx, inviteID, InviteRevoked); err != nil {
		return apperrors.Internal(fmt.Errorf("revoke invite: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &orgID, ActorID: &actor.UserID, Action: "organization.invite_revoked",
		ResourceType: strPtr("organization_invite"), ResourceID: &inviteID,
	})
	return nil
}

func (s *service) AcceptInvite(ctx context.Context, actor domain.Actor, identifier string) (*Membership, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return nil, apperrors.Validation("organization.invalid_input", "token is required", nil)
	}

	// A UUID identifies the invite directly (the live-notification flow,
	// ListMyInvites) — anything else is treated as the raw token (the
	// copy-a-link fallback). Never ambiguous: a raw token is 43 base64url
	// characters with no dashes, nothing a UUID could ever parse as.
	var inv *Invite
	var err error
	if inviteID, parseErr := uuid.Parse(identifier); parseErr == nil {
		inv, err = s.invites.GetByID(ctx, inviteID)
	} else {
		inv, err = s.invites.GetByTokenHash(ctx, hashInviteToken(identifier))
	}
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("organization.invite_not_found", "invite not found or already used")
		}
		return nil, apperrors.Internal(fmt.Errorf("get invite: %w", err))
	}

	return s.acceptInvite(ctx, actor, inv)
}

// acceptInvite is AcceptInvite's shared tail once the invite row has been
// resolved (by ID or by token) — status/expiry/email-match/membership-
// creation/audit, identical regardless of which identifier the caller used.
func (s *service) acceptInvite(ctx context.Context, actor domain.Actor, inv *Invite) (*Membership, error) {
	if inv.Status != InvitePending {
		return nil, apperrors.Unprocessable("organization.invite_not_pending", "this invite is no longer valid")
	}
	if inv.ExpiresAt.Before(time.Now().UTC()) {
		_ = s.invites.SetStatus(ctx, inv.ID, InviteExpired)
		return nil, apperrors.Unprocessable("organization.invite_expired", "this invite has expired")
	}

	caller, err := s.users.GetInfoByID(ctx, actor.UserID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("get caller: %w", err))
	}
	if !strings.EqualFold(caller.Email, inv.Email) {
		return nil, apperrors.Forbidden("organization.invite_email_mismatch", "this invite was sent to a different email address — log in with that account to accept it")
	}
	if already, err := s.isAlreadyMember(ctx, inv.OrgID, caller.Email); err != nil {
		return nil, err
	} else if already {
		return nil, apperrors.Conflict("organization.already_member", "you are already a member of this organization")
	}

	m := &Membership{ID: id.New(), OrgID: inv.OrgID, UserID: actor.UserID, Role: inv.Role, InvitedBy: inv.InvitedBy}
	if err := s.memberships.Create(ctx, m); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("create membership: %w", err))
	}
	if err := s.invites.SetStatus(ctx, inv.ID, InviteAccepted); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("mark invite accepted: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &inv.OrgID, ActorID: &actor.UserID, Action: "organization.invite_accepted",
		ResourceType: strPtr("organization_invite"), ResourceID: &inv.ID,
	})
	return m, nil
}

// ListMyInvites is the live notification feed's own read — see the Service
// interface's own doc comment.
func (s *service) ListMyInvites(ctx context.Context, actor domain.Actor) ([]PendingInvite, error) {
	caller, err := s.users.GetInfoByID(ctx, actor.UserID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("get caller: %w", err))
	}
	invites, err := s.invites.ListByEmail(ctx, caller.Email)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list invites by email: %w", err))
	}

	now := time.Now().UTC()
	out := make([]PendingInvite, 0, len(invites))
	for _, inv := range invites {
		if inv.Status != InvitePending {
			continue
		}
		if inv.ExpiresAt.Before(now) {
			_ = s.invites.SetStatus(ctx, inv.ID, InviteExpired)
			continue
		}
		orgName, err := s.orgs.GetName(ctx, inv.OrgID)
		if err != nil {
			return nil, apperrors.Internal(fmt.Errorf("get organization name: %w", err))
		}
		out = append(out, PendingInvite{Invite: inv, OrgName: orgName})
	}
	return out, nil
}

func (s *service) DeclineInvite(ctx context.Context, actor domain.Actor, inviteID uuid.UUID) error {
	inv, err := s.invites.GetByID(ctx, inviteID)
	if err != nil {
		if isNotFound(err) {
			return apperrors.NotFound("organization.invite_not_found", "invite not found")
		}
		return apperrors.Internal(fmt.Errorf("get invite: %w", err))
	}
	if inv.Status != InvitePending {
		return apperrors.Unprocessable("organization.invite_not_pending", "this invite is no longer pending")
	}

	caller, err := s.users.GetInfoByID(ctx, actor.UserID)
	if err != nil {
		return apperrors.Internal(fmt.Errorf("get caller: %w", err))
	}
	if !strings.EqualFold(caller.Email, inv.Email) {
		return apperrors.Forbidden("organization.invite_email_mismatch", "this invite was sent to a different email address")
	}

	if err := s.invites.SetStatus(ctx, inviteID, InviteDeclined); err != nil {
		return apperrors.Internal(fmt.Errorf("decline invite: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &inv.OrgID, ActorID: &actor.UserID, Action: "organization.invite_declined",
		ResourceType: strPtr("organization_invite"), ResourceID: &inviteID,
	})
	return nil
}

func (s *service) SwitchOrg(ctx context.Context, actor domain.Actor, targetOrgID uuid.UUID) (*SwitchOrgResult, error) {
	u, err := s.users.GetInfoByID(ctx, actor.UserID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("get caller: %w", err))
	}

	var role domain.Role
	if targetOrgID == u.OrgID {
		role = u.Role
	} else {
		m, err := s.memberships.GetByOrgAndUser(ctx, targetOrgID, actor.UserID)
		if err != nil {
			if isNotFound(err) {
				return nil, apperrors.Forbidden("organization.not_a_member", "you are not a member of this organization")
			}
			return nil, apperrors.Internal(fmt.Errorf("get membership: %w", err))
		}
		role = m.Role
	}

	pair, err := s.identitySvc.IssueTokenPairForOrg(ctx, actor.UserID, targetOrgID, role)
	if err != nil {
		return nil, err
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &targetOrgID, ActorID: &actor.UserID, Action: "auth.org_switched",
	})
	return &SwitchOrgResult{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken, ExpiresIn: pair.ExpiresIn}, nil
}

func (s *service) ListMemberOrgs(ctx context.Context, actor domain.Actor) ([]MemberOrgSummary, error) {
	u, err := s.users.GetInfoByID(ctx, actor.UserID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("get caller: %w", err))
	}
	homeName, err := s.orgs.GetName(ctx, u.OrgID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("get home organization name: %w", err))
	}
	out := []MemberOrgSummary{{OrgID: u.OrgID, Name: homeName, Role: u.Role, IsHome: true}}

	memberships, err := s.memberships.ListByUser(ctx, actor.UserID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list memberships: %w", err))
	}
	for _, m := range memberships {
		name, err := s.orgs.GetName(ctx, m.OrgID)
		if err != nil {
			return nil, apperrors.Internal(fmt.Errorf("get organization name: %w", err))
		}
		out = append(out, MemberOrgSummary{OrgID: m.OrgID, Name: name, Role: m.Role, IsHome: false})
	}
	return out, nil
}

// resolveMember looks a member up by either their home-org row (users.org_id
// == orgID) or an organization_memberships row — the same "check the more
// specific/primary source, fall back to the join table" shape ListMembers
// uses, just for a single member.
func (s *service) resolveMember(ctx context.Context, orgID, userID uuid.UUID) (isHome bool, role domain.Role, err error) {
	u, err := s.users.GetInfoByID(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return false, "", apperrors.NotFound("organization.member_not_found", "member not found in this organization")
		}
		return false, "", apperrors.Internal(fmt.Errorf("get user: %w", err))
	}
	if u.OrgID == orgID {
		return true, u.Role, nil
	}
	m, err := s.memberships.GetByOrgAndUser(ctx, orgID, userID)
	if err != nil {
		if isNotFound(err) {
			return false, "", apperrors.NotFound("organization.member_not_found", "member not found in this organization")
		}
		return false, "", apperrors.Internal(fmt.Errorf("get membership: %w", err))
	}
	return false, m.Role, nil
}

// requireNotLastAdmin implements the last-Owner constraint — see the
// Service interface's own doc comment. Called only when the change under
// consideration would take an existing admin away from admin (demote or
// remove); a promotion, or a change that leaves the role unchanged, never
// needs this check.
func (s *service) requireNotLastAdmin(ctx context.Context, orgID uuid.UUID) error {
	home, err := s.users.ListHomeMembers(ctx, orgID)
	if err != nil {
		return apperrors.Internal(fmt.Errorf("list home members: %w", err))
	}
	memberships, err := s.memberships.ListByOrg(ctx, orgID)
	if err != nil {
		return apperrors.Internal(fmt.Errorf("list memberships: %w", err))
	}
	admins := 0
	for _, u := range home {
		if u.Role == domain.RoleAdmin {
			admins++
		}
	}
	for _, m := range memberships {
		if m.Role == domain.RoleAdmin {
			admins++
		}
	}
	if admins <= 1 {
		return apperrors.Validation("organization.last_admin", "the organization must always have at least one remaining admin", nil)
	}
	return nil
}

func (s *service) isAlreadyMember(ctx context.Context, orgID uuid.UUID, email string) (bool, error) {
	home, err := s.users.ListHomeMembers(ctx, orgID)
	if err != nil {
		return false, apperrors.Internal(fmt.Errorf("list home members: %w", err))
	}
	for _, u := range home {
		if strings.EqualFold(u.Email, email) {
			return true, nil
		}
	}
	memberships, err := s.memberships.ListByOrg(ctx, orgID)
	if err != nil {
		return false, apperrors.Internal(fmt.Errorf("list memberships: %w", err))
	}
	for _, m := range memberships {
		u, err := s.users.GetInfoByID(ctx, m.UserID)
		if err != nil {
			return false, apperrors.Internal(fmt.Errorf("get member user: %w", err))
		}
		if strings.EqualFold(u.Email, email) {
			return true, nil
		}
	}
	return false, nil
}

// requireOwnOrg is the standing "404, not 403, for cross-org" rule
// (documentation/07-api-specification.md §1.4, project.getOwnedProject's
// own precedent) applied to an org path parameter instead of a resource ID
// — confirming another organization's existence by name is itself a leak.
func requireOwnOrg(actor domain.Actor, orgID uuid.UUID) error {
	if actor.OrgID != orgID {
		return apperrors.NotFound("organization.not_found", "organization not found")
	}
	return nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func newInviteToken() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate random bytes: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashInviteToken(raw), nil
}

func hashInviteToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func strPtr(s string) *string { return &s }

func isNotFound(err error) bool {
	var appErr *apperrors.Error
	return errors.As(err, &appErr) && appErr.Kind == apperrors.KindNotFound
}
