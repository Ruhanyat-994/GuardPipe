package identity

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

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/crypto"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// maxFailedLogins and lockoutDuration implement the account-lockout half of
// the `users` table's `failed_login_count`/`locked_until` columns
// (documentation/06-database-design.md §4.2). Neither value is specified by
// any requirement doc — FR-IAM-009 only mandates the 5/min/IP rate limit —
// so these are a conservative default layered on top of it, not a
// documented contract. Revisit if a specific threshold gets specified.
const (
	maxFailedLogins = 10
	lockoutDuration = 15 * time.Minute
)

// Service is authentication and access control
// (documentation/05-module-specifications.md §3).
type Service interface {
	Register(ctx context.Context, in RegisterInput) (*User, error)
	Login(ctx context.Context, email, password string) (*TokenPair, error)
	Refresh(ctx context.Context, refreshToken string) (*TokenPair, error)
	Logout(ctx context.Context, refreshToken string) error
	Verify(ctx context.Context, accessToken string) (*Claims, error)
	Me(ctx context.Context, actor domain.Actor) (*User, error)
	// CheckSuspension is BUILD_GUIDE.md Phase 14's per-request suspension
	// gate (middleware.SuspensionCheck) — checked on every authenticated
	// request, not just at Login, so an operator suspending an account or
	// organisation takes effect immediately rather than waiting out the
	// access token's TTL. Returns a *platform/errors.Error (Forbidden,
	// "auth.account_suspended") when either the user or their organisation
	// is currently suspended, nil otherwise.
	CheckSuspension(ctx context.Context, actor domain.Actor) error

	// IssueTokenPairForOrg is BUILD_GUIDE.md Phase 15's `POST /auth/switch-org`
	// primitive — mints a brand-new token pair (its own rotation family, independent
	// of whatever family the caller is currently using) scoped to orgID/role
	// rather than the user's home organisation. The caller (modules/organization)
	// has already verified userID actually holds role in orgID — this method does
	// not re-check membership, only that the user/org aren't suspended, the same
	// gate Login already applies.
	IssueTokenPairForOrg(ctx context.Context, userID, orgID uuid.UUID, role domain.Role) (*TokenPair, error)

	// IssueTokenPairForProject is the project-collaborators follow-up's
	// `POST /auth/switch-project/{id}` primitive — same shape as
	// IssueTokenPairForOrg (brand-new independent rotation family, no
	// re-verification here), but the resulting token is scoped to exactly
	// projectID (domain.Actor.ProjectID), not the whole of orgID. The
	// caller (transport, via modules/project.GetCollaboratorGrant) has
	// already resolved orgID/role from an accepted project_collaborators row.
	IssueTokenPairForProject(ctx context.Context, userID, projectID, orgID uuid.UUID, role domain.Role) (*TokenPair, error)
}

// UserRepository is defined by this package (the consumer), per
// documentation/04-backend-architecture.md §5.1 — its implementation lives
// in internal/store/repo.
type UserRepository interface {
	Create(ctx context.Context, u *User) error
	CountAll(ctx context.Context) (int, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*User, error)
	SetFailedLogin(ctx context.Context, id uuid.UUID, count int, lockedUntil *time.Time) error
	RecordSuccessfulLogin(ctx context.Context, id uuid.UUID, loginAt time.Time) error
}

// OrganizationRepository — each registration creates its own organisation
// (documentation/06-database-design.md §4.1); GuardPipe is multi-tenant, not
// one shared workspace. See PROGRESS-LOG.md for why the earlier
// single-shared-organisation model (GetSole/EnsureDefault) was replaced.
type OrganizationRepository interface {
	Create(ctx context.Context, name string) (uuid.UUID, error)
	// GetSuspensionState reads organizations.suspended_at/suspended_reason
	// (migration 00017) — written only by modules/admin, through this
	// module's own OrganizationRepository.SetSuspended (identity.Service
	// exposes no way to suspend an organisation itself; that decision lives
	// in modules/admin). reason is "" when suspendedAt is nil.
	GetSuspensionState(ctx context.Context, orgID uuid.UUID) (suspendedAt *time.Time, reason string, err error)
}

// RefreshTokenRepository is defined by this package; implementation in
// internal/store/repo.
type RefreshTokenRepository interface {
	Create(ctx context.Context, rt *RefreshToken) error
	GetByHash(ctx context.Context, tokenHash string) (*RefreshToken, error)
	MarkConsumed(ctx context.Context, id uuid.UUID, consumedAt time.Time) error
	RevokeFamily(ctx context.Context, familyID uuid.UUID, revokedAt time.Time) error
}

// MembershipRoleReader is the one thing this package needs from
// modules/organization (BUILD_GUIDE.md Phase 15) — implemented in
// internal/store/repo against organization_memberships, the table that
// module owns; identity only ever reads it through this narrow interface,
// never writes it (the same "define the interface you need, not the whole
// module" pattern project.UserDisplayNameLookup already establishes against
// identity). It's what lets Refresh rebuild the correct role for a
// non-home-org-scoped refresh token (RefreshToken.OrgID) instead of only
// ever knowing about the user's own home-org role.
type MembershipRoleReader interface {
	// GetRole returns the caller's role within orgID via
	// organization_memberships, or a NotFound *platform/errors.Error if
	// userID does not currently hold a membership in orgID.
	GetRole(ctx context.Context, orgID, userID uuid.UUID) (domain.Role, error)
}

// ProjectCollaboratorRoleReader is the project-collaborators follow-up's
// analogue of MembershipRoleReader — implemented in internal/store/repo
// against project_collaborators, a table modules/project owns; identity
// only ever reads it through this narrow interface. Lets Refresh rebuild a
// project-scoped session's current role (and the project's owning org,
// needed to rebuild orgID too) on every rotation, instead of only ever
// trusting what the original switch-project call baked in.
type ProjectCollaboratorRoleReader interface {
	// GetRoleAndOrg returns the caller's current role for projectID via
	// project_collaborators, plus the project's owning org id, or a
	// NotFound *platform/errors.Error if userID holds no accepted grant on
	// projectID.
	GetRoleAndOrg(ctx context.Context, projectID, userID uuid.UUID) (domain.Role, uuid.UUID, error)
}

type service struct {
	users              UserRepository
	orgs               OrganizationRepository
	tokens             RefreshTokenRepository
	issuer             *TokenIssuer
	audit              audit.Service
	accessTokenTTL     time.Duration
	refreshTokenTTL    time.Duration
	sessionAbsoluteTTL time.Duration
	// membershipRoles is BUILD_GUIDE.md Phase 15's MembershipRoleReader — may
	// be nil (a test that never exercises a non-home-org refresh doesn't need
	// one), but is always wired in production. Never consulted by Login;
	// Login only ever issues a home-org token pair.
	membershipRoles MembershipRoleReader
	// projectRoles is the project-collaborators follow-up's
	// ProjectCollaboratorRoleReader — same nilable-in-tests, always-wired-
	// in-production contract as membershipRoles. Never consulted by Login;
	// only by Refresh (for an already-switched-into-a-project session) and
	// IssueTokenPairForProject.
	projectRoles ProjectCollaboratorRoleReader
}

// NewService wires the identity module. accessTokenTTL/refreshTokenTTL/
// sessionAbsoluteTTL come from platform/config
// (GUARDPIPE_ACCESS_TOKEN_TTL/GUARDPIPE_REFRESH_TOKEN_TTL/GUARDPIPE_SESSION_ABSOLUTE_TTL).
// refreshTokenTTL is the *idle* timeout — Refresh resets it forward on every
// rotation, so a session that keeps getting used never hits it.
// sessionAbsoluteTTL (BUILD_GUIDE.md Phase 14) is the ceiling on top of
// that: Refresh also rejects once the token's whole family — traced back to
// the original login via RefreshToken.FamilyIssuedAt, unchanged across every
// rotation — is older than this, regardless of activity. auditSvc is
// BUILD_GUIDE.md Phase 6's retroactive instrumentation —
// login/logout/refresh-reuse-detected are the three events named there.
// membershipRoles (BUILD_GUIDE.md Phase 15) may be nil — a caller that never
// exercises a non-home-org refresh (most existing tests) doesn't need one;
// cmd/guardpipe/main.go always wires a real one in production.
func NewService(
	users UserRepository,
	orgs OrganizationRepository,
	tokens RefreshTokenRepository,
	issuer *TokenIssuer,
	auditSvc audit.Service,
	accessTokenTTL, refreshTokenTTL, sessionAbsoluteTTL time.Duration,
	membershipRoles MembershipRoleReader,
	projectRoles ProjectCollaboratorRoleReader,
) Service {
	return &service{
		users:              users,
		orgs:               orgs,
		tokens:             tokens,
		issuer:             issuer,
		audit:              auditSvc,
		accessTokenTTL:     accessTokenTTL,
		refreshTokenTTL:    refreshTokenTTL,
		sessionAbsoluteTTL: sessionAbsoluteTTL,
		membershipRoles:    membershipRoles,
		projectRoles:       projectRoles,
	}
}

func (s *service) Register(ctx context.Context, in RegisterInput) (*User, error) {
	email := normalizeEmail(in.Email)
	displayName := strings.TrimSpace(in.DisplayName)
	if email == "" || displayName == "" {
		return nil, apperrors.Validation("identity.invalid_input", "email and display name are required", nil)
	}
	if err := validatePassword(in.Password); err != nil {
		return nil, err
	}

	if _, err := s.users.GetByEmail(ctx, email); err == nil {
		return nil, apperrors.Conflict("identity.email_taken", "an account with this email already exists")
	} else if !isNotFound(err) {
		return nil, apperrors.Internal(fmt.Errorf("check existing email: %w", err))
	}

	passwordHash, err := crypto.HashPassword(in.Password)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("hash password: %w", err))
	}

	// Every registration gets its own new organisation — GuardPipe is
	// multi-tenant, each account is its own isolated workspace, not a seat
	// in one shared company workspace. The registrant is that organisation's
	// only member, so they're always its admin (documentation/05-module-specifications.md
	// §3, roles table — "admin" is "founding/only member of this org," not
	// "first person to ever use the product").
	orgID, err := s.orgs.Create(ctx, orgNameFor(displayName))
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("create organisation: %w", err))
	}

	user := &User{
		ID:           id.New(),
		OrgID:        orgID,
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: passwordHash,
		Role:         domain.RoleAdmin,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("create user: %w", err))
	}
	return user, nil
}

func (s *service) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	email = normalizeEmail(email)

	user, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		if isNotFound(err) {
			// Run Argon2id anyway so the response time for "unknown email"
			// matches "wrong password" — otherwise the timing difference is
			// a user-enumeration side channel
			// (documentation/12-security-and-threat-model.md, finding I9).
			_, _ = crypto.VerifyPassword(password, crypto.DummyHash)
			return nil, errInvalidCredentials()
		}
		return nil, apperrors.Internal(fmt.Errorf("get user by email: %w", err))
	}

	if user.LockedUntil != nil && user.LockedUntil.After(time.Now().UTC()) {
		// Same message as a wrong password — the lock itself is not
		// user-visible information (no enumeration of account state).
		return nil, errInvalidCredentials()
	}
	// Unlike a lockout, a platform-operator suspension IS meant to be
	// visible to the account holder (BUILD_GUIDE.md Phase 14) — it isn't a
	// security-through-obscurity mechanism, it's an operational action with
	// a stated reason. Checked here before spending an Argon2id verify on a
	// password that, even if correct, won't result in a usable session.
	if user.SuspendedAt != nil {
		return nil, suspendedError(user.SuspendedReason)
	}
	if orgSuspendedAt, orgReason, err := s.orgs.GetSuspensionState(ctx, user.OrgID); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("get organization suspension state: %w", err))
	} else if orgSuspendedAt != nil {
		return nil, suspendedError(&orgReason)
	}

	ok, err := crypto.VerifyPassword(password, user.PasswordHash)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("verify password: %w", err))
	}
	if !ok {
		newCount := user.FailedLoginCount + 1
		var lockedUntil *time.Time
		if newCount >= maxFailedLogins {
			until := time.Now().UTC().Add(lockoutDuration)
			lockedUntil = &until
		}
		if err := s.users.SetFailedLogin(ctx, user.ID, newCount, lockedUntil); err != nil {
			return nil, apperrors.Internal(fmt.Errorf("record failed login: %w", err))
		}
		return nil, errInvalidCredentials()
	}

	if err := s.users.RecordSuccessfulLogin(ctx, user.ID, time.Now().UTC()); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("record successful login: %w", err))
	}

	s.audit.Log(ctx, audit.Entry{OrgID: &user.OrgID, ActorID: &user.ID, Action: "auth.login"})

	return s.issueTokenPair(ctx, user)
}

func (s *service) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	rt, err := s.tokens.GetByHash(ctx, hashRefreshToken(refreshToken))
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.Unauthorized("auth.token_invalid", "refresh token is invalid")
		}
		return nil, apperrors.Internal(fmt.Errorf("get refresh token: %w", err))
	}

	now := time.Now().UTC()
	if rt.RevokedAt != nil {
		return nil, apperrors.Unauthorized("auth.token_invalid", "refresh token has been revoked")
	}
	if rt.ConsumedAt != nil {
		// Reuse detection: a consumed token presented again is theft
		// evidence — invalidate the whole family
		// (documentation/05-module-specifications.md §3).
		_ = s.tokens.RevokeFamily(ctx, rt.FamilyID, now)
		userID := rt.UserID
		s.audit.Log(ctx, audit.Entry{
			ActorID: &userID, Action: "auth.refresh_reused",
			Detail: map[string]any{"family_id": rt.FamilyID.String()},
		})
		return nil, apperrors.Unauthorized("auth.refresh_reused", "refresh token was already used; all sessions on this device have been revoked")
	}
	if rt.ExpiresAt.Before(now) {
		return nil, apperrors.Unauthorized("auth.token_expired", "refresh token has expired")
	}
	// Absolute session cap (BUILD_GUIDE.md Phase 14): independent of the
	// idle-timeout check above, a session traces back to FamilyIssuedAt —
	// the original login — and dies at sessionAbsoluteTTL regardless of how
	// recently it was refreshed. Revoking the family here (not just
	// rejecting this one call) means the very next presented token in the
	// chain fails the same way, rather than leaving a technically-still-live
	// row a caller could otherwise keep probing.
	if now.Sub(rt.FamilyIssuedAt) > s.sessionAbsoluteTTL {
		_ = s.tokens.RevokeFamily(ctx, rt.FamilyID, now)
		userID := rt.UserID
		s.audit.Log(ctx, audit.Entry{
			ActorID: &userID, Action: "auth.session_expired",
			Detail: map[string]any{"family_id": rt.FamilyID.String()},
		})
		return nil, apperrors.Unauthorized("auth.session_expired", "session has exceeded its maximum lifetime; please log in again")
	}

	if err := s.tokens.MarkConsumed(ctx, rt.ID, now); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("consume refresh token: %w", err))
	}

	user, err := s.users.GetByID(ctx, rt.UserID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("get user for refresh: %w", err))
	}

	// orgID/role default to the user's home organisation — the behaviour
	// every refresh token issued before migration 00022 (rt.OrgID nil) and
	// every ordinary login already has. A switch-org'd token pair
	// (rt.OrgID set, BUILD_GUIDE.md Phase 15) instead rebuilds against that
	// org context, resolving the role via membershipRoles rather than
	// user.Role whenever it isn't the home org.
	orgID, role := user.OrgID, user.Role
	switch {
	case rt.ProjectID != nil:
		// A project-scoped session (switch-project) — re-resolve both role
		// and org fresh from project_collaborators every rotation, the same
		// "never just trust what the original token baked in" rule the
		// org-switch branch below already follows, so a revoked collaborator
		// grant kills the session on its very next refresh.
		if s.projectRoles == nil {
			return nil, apperrors.Internal(errors.New("identity: refresh token carries a project scope but no ProjectCollaboratorRoleReader is wired"))
		}
		role, orgID, err = s.projectRoles.GetRoleAndOrg(ctx, *rt.ProjectID, user.ID)
		if err != nil {
			if isNotFound(err) {
				_ = s.tokens.RevokeFamily(ctx, rt.FamilyID, now)
				return nil, apperrors.Unauthorized("auth.token_invalid", "you no longer have access to this shared project")
			}
			return nil, apperrors.Internal(fmt.Errorf("resolve project collaborator role for refresh: %w", err))
		}
	case rt.OrgID != nil && *rt.OrgID != user.OrgID:
		orgID = *rt.OrgID
		if s.membershipRoles == nil {
			return nil, apperrors.Internal(errors.New("identity: refresh token carries a non-home org context but no MembershipRoleReader is wired"))
		}
		role, err = s.membershipRoles.GetRole(ctx, orgID, user.ID)
		if err != nil {
			if isNotFound(err) {
				// The membership backing this session was removed since it
				// was issued (kicked from the org) — the whole family dies,
				// not just this one refresh, so a stale token can't keep
				// being retried against an org the user no longer belongs to.
				_ = s.tokens.RevokeFamily(ctx, rt.FamilyID, now)
				return nil, apperrors.Unauthorized("auth.token_invalid", "you are no longer a member of this organization")
			}
			return nil, apperrors.Internal(fmt.Errorf("resolve membership role for refresh: %w", err))
		}
	}

	pair, err := s.issueTokenPairInFamily(ctx, user, orgID, role, rt.FamilyID, rt.FamilyIssuedAt, rt.ProjectID)
	if err != nil {
		return nil, err
	}
	pair.User = nil // the refresh response doesn't repeat the user object
	return pair, nil
}

func (s *service) Logout(ctx context.Context, refreshToken string) error {
	rt, err := s.tokens.GetByHash(ctx, hashRefreshToken(refreshToken))
	if err != nil {
		if isNotFound(err) {
			return nil // already gone — logout is idempotent
		}
		return apperrors.Internal(fmt.Errorf("get refresh token: %w", err))
	}
	if err := s.tokens.RevokeFamily(ctx, rt.FamilyID, time.Now().UTC()); err != nil {
		return apperrors.Internal(fmt.Errorf("revoke token family: %w", err))
	}
	userID := rt.UserID
	s.audit.Log(ctx, audit.Entry{ActorID: &userID, Action: "auth.logout"})
	return nil
}

func (s *service) Verify(ctx context.Context, accessToken string) (*Claims, error) {
	claims, err := s.issuer.Parse(accessToken)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, apperrors.Unauthorized("auth.token_expired", "access token has expired")
		}
		return nil, apperrors.Unauthorized("auth.token_invalid", "access token is invalid")
	}
	return claims, nil
}

func (s *service) CheckSuspension(ctx context.Context, actor domain.Actor) error {
	user, err := s.users.GetByID(ctx, actor.UserID)
	if err != nil {
		if isNotFound(err) {
			return apperrors.Unauthorized("auth.token_invalid", "account no longer exists")
		}
		return apperrors.Internal(fmt.Errorf("get user for suspension check: %w", err))
	}
	if user.SuspendedAt != nil {
		return suspendedError(user.SuspendedReason)
	}

	suspendedAt, reason, err := s.orgs.GetSuspensionState(ctx, actor.OrgID)
	if err != nil {
		return apperrors.Internal(fmt.Errorf("get organization suspension state: %w", err))
	}
	if suspendedAt != nil {
		return suspendedError(&reason)
	}
	return nil
}

func (s *service) Me(ctx context.Context, actor domain.Actor) (*User, error) {
	user, err := s.users.GetByID(ctx, actor.UserID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("identity.user_not_found", "user not found")
		}
		return nil, apperrors.Internal(fmt.Errorf("get user: %w", err))
	}
	return user, nil
}

func (s *service) issueTokenPair(ctx context.Context, user *User) (*TokenPair, error) {
	now := time.Now().UTC()
	return s.issueTokenPairInFamily(ctx, user, user.OrgID, user.Role, id.New(), now, nil)
}

// issueTokenPairInFamily issues a token pair within an existing rotation
// family, scoped to orgID/role — ordinarily user.OrgID/user.Role (the
// caller's home org), but BUILD_GUIDE.md Phase 15's switch-org and Refresh
// (for an already-switched session) pass a non-home org context instead.
// familyIssuedAt is the family's original login time — unchanged across
// every rotation — carried forward so the absolute session cap in Refresh
// has something to measure against without a second query. projectID is nil
// for every ordinary/switch-org pair; non-nil only for the project-
// collaborators follow-up's switch-project pair (and Refresh rebuilding one).
func (s *service) issueTokenPairInFamily(ctx context.Context, user *User, orgID uuid.UUID, role domain.Role, familyID uuid.UUID, familyIssuedAt time.Time, projectID *uuid.UUID) (*TokenPair, error) {
	accessToken, err := s.issuer.Issue(user.ID, orgID, role, projectID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("issue access token: %w", err))
	}

	rawRefresh, tokenHash, err := newRefreshToken()
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("generate refresh token: %w", err))
	}

	rt := &RefreshToken{
		ID:             id.New(),
		UserID:         user.ID,
		TokenHash:      tokenHash,
		FamilyID:       familyID,
		FamilyIssuedAt: familyIssuedAt,
		ExpiresAt:      time.Now().UTC().Add(s.refreshTokenTTL),
		OrgID:          &orgID,
		ProjectID:      projectID,
	}
	if err := s.tokens.Create(ctx, rt); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("store refresh token: %w", err))
	}

	// The response User always reflects the org context this token pair is
	// actually scoped to (Login's home-org pair unaffected: orgID/role are
	// already user.OrgID/user.Role there), not always the account's literal
	// home-org row — a switch-org'd caller's `/auth/me`-shaped User.Role
	// should read as their role in the org they're now acting as.
	responseUser := *user
	responseUser.OrgID, responseUser.Role = orgID, role
	responseUser.ScopedProjectID = projectID

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		ExpiresIn:    int(s.accessTokenTTL.Seconds()),
		User:         &responseUser,
	}, nil
}

// IssueTokenPairForOrg is BUILD_GUIDE.md Phase 15's switch-org primitive —
// see the Service interface's own doc comment. Always starts a brand-new
// rotation family (id.New()/now), independent of any family the caller is
// currently using — a member of two orgs holds two independent sessions,
// never a blended one, matching CLAUDE.md's stated multi-membership model.
func (s *service) IssueTokenPairForOrg(ctx context.Context, userID, orgID uuid.UUID, role domain.Role) (*TokenPair, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.Unauthorized("auth.token_invalid", "account no longer exists")
		}
		return nil, apperrors.Internal(fmt.Errorf("get user for org switch: %w", err))
	}
	if user.SuspendedAt != nil {
		return nil, suspendedError(user.SuspendedReason)
	}
	if suspendedAt, reason, err := s.orgs.GetSuspensionState(ctx, orgID); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("get organization suspension state: %w", err))
	} else if suspendedAt != nil {
		return nil, suspendedError(&reason)
	}

	now := time.Now().UTC()
	return s.issueTokenPairInFamily(ctx, user, orgID, role, id.New(), now, nil)
}

// IssueTokenPairForProject is the project-collaborators follow-up's
// switch-project primitive — see the Service interface's own doc comment.
// Like IssueTokenPairForOrg, always starts a brand-new rotation family,
// independent of the caller's home session; unlike it, the resulting token
// is also stamped with projectID, so every downstream authorisation check
// (project.service.getOwnedProject) refuses any other project in orgID even
// though the token's own org_id/role claims are otherwise indistinguishable
// from an ordinary member of that org. identity itself does not verify the
// caller actually holds a collaborator grant on projectID — the transport
// handler resolves projectID/orgID/role from an accepted project_collaborators
// row (via modules/project) before ever calling this; identity knows
// nothing about projects (this package's own doc comment).
func (s *service) IssueTokenPairForProject(ctx context.Context, userID, projectID, orgID uuid.UUID, role domain.Role) (*TokenPair, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.Unauthorized("auth.token_invalid", "account no longer exists")
		}
		return nil, apperrors.Internal(fmt.Errorf("get user for project switch: %w", err))
	}
	if user.SuspendedAt != nil {
		return nil, suspendedError(user.SuspendedReason)
	}
	if suspendedAt, reason, err := s.orgs.GetSuspensionState(ctx, orgID); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("get organization suspension state: %w", err))
	} else if suspendedAt != nil {
		return nil, suspendedError(&reason)
	}

	now := time.Now().UTC()
	return s.issueTokenPairInFamily(ctx, user, orgID, role, id.New(), now, &projectID)
}

func newRefreshToken() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate random bytes: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashRefreshToken(raw), nil
}

func hashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// orgNameFor derives a default organisation name at registration time — the
// register form only collects email/display name/password (no separate "org
// name" field, keeping signup a single step), so this is a reasonable
// starting name the user can rename later once account settings exist.
func orgNameFor(displayName string) string {
	return displayName + "'s Organization"
}

func errInvalidCredentials() error {
	return apperrors.Unauthorized("auth.invalid_credentials", "email or password is incorrect")
}

// suspendedError is BUILD_GUIDE.md Phase 14's rejection for a suspended
// user or organisation — Forbidden (403), not Unauthorized (401): the
// credential itself is valid, the account is simply not permitted to act
// right now, and the reason is meant to be visible (see Login's own
// comment on this).
func suspendedError(reason *string) error {
	detail := "this account has been suspended"
	if reason != nil && *reason != "" {
		detail = "this account has been suspended: " + *reason
	}
	return apperrors.Forbidden("auth.account_suspended", detail)
}

func isNotFound(err error) bool {
	var appErr *apperrors.Error
	return errors.As(err, &appErr) && appErr.Kind == apperrors.KindNotFound
}
