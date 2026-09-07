package identity_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
)

// --- hand-written fakes (no mocking framework) ---

// fakeAuditService records every entry logged, so tests can assert
// BUILD_GUIDE.md Phase 6's login/logout/refresh-reuse-detected
// instrumentation actually fires, without needing a real audit_log table.
type fakeAuditService struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (f *fakeAuditService) Log(_ context.Context, e audit.Entry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
}

func (f *fakeAuditService) List(_ context.Context, _ audit.ListFilter, _ audit.Page) ([]audit.Entry, int, error) {
	return nil, 0, nil
}

func (f *fakeAuditService) actions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.entries))
	for i, e := range f.entries {
		out[i] = e.Action
	}
	return out
}

type fakeUserRepo struct {
	mu    sync.Mutex
	byID  map[uuid.UUID]*identity.User
	orgID uuid.UUID
}

func newFakeUserRepo(orgID uuid.UUID) *fakeUserRepo {
	return &fakeUserRepo{byID: map[uuid.UUID]*identity.User{}, orgID: orgID}
}

func (f *fakeUserRepo) Create(_ context.Context, u *identity.User) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[u.ID] = u
	return nil
}

func (f *fakeUserRepo) CountAll(_ context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.byID), nil
}

func (f *fakeUserRepo) GetByEmail(_ context.Context, email string) (*identity.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, u := range f.byID {
		if strings.EqualFold(u.Email, email) {
			cp := *u
			return &cp, nil
		}
	}
	return nil, apperrors.NotFound("identity.user_not_found", "no such user")
}

func (f *fakeUserRepo) GetByID(_ context.Context, id uuid.UUID) (*identity.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[id]
	if !ok {
		return nil, apperrors.NotFound("identity.user_not_found", "no such user")
	}
	cp := *u
	return &cp, nil
}

func (f *fakeUserRepo) SetFailedLogin(_ context.Context, id uuid.UUID, count int, lockedUntil *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[id]
	if !ok {
		return apperrors.NotFound("identity.user_not_found", "no such user")
	}
	u.FailedLoginCount = count
	u.LockedUntil = lockedUntil
	return nil
}

func (f *fakeUserRepo) RecordSuccessfulLogin(_ context.Context, id uuid.UUID, loginAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.byID[id]
	if !ok {
		return apperrors.NotFound("identity.user_not_found", "no such user")
	}
	u.FailedLoginCount = 0
	u.LockedUntil = nil
	u.LastLoginAt = &loginAt
	return nil
}

// fakeOrgRepo mimics the real repo: every Create call makes a brand-new
// organisation id, exactly like real registrations each getting their own
// isolated org (no shared "sole" organisation any more).
type fakeOrgRepo struct {
	mu        sync.Mutex
	created   []string // names, for assertions that care
	suspended map[uuid.UUID]string
}

func (f *fakeOrgRepo) Create(_ context.Context, name string) (uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = append(f.created, name)
	return id.New(), nil
}

func (f *fakeOrgRepo) GetSuspensionState(_ context.Context, orgID uuid.UUID) (*time.Time, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reason, ok := f.suspended[orgID]
	if !ok {
		return nil, "", nil
	}
	now := time.Now().UTC()
	return &now, reason, nil
}

type fakeTokenRepo struct {
	mu     sync.Mutex
	byHash map[string]*identity.RefreshToken
}

func newFakeTokenRepo() *fakeTokenRepo {
	return &fakeTokenRepo{byHash: map[string]*identity.RefreshToken{}}
}

func (f *fakeTokenRepo) Create(_ context.Context, rt *identity.RefreshToken) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *rt
	f.byHash[rt.TokenHash] = &cp
	return nil
}

func (f *fakeTokenRepo) GetByHash(_ context.Context, tokenHash string) (*identity.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	rt, ok := f.byHash[tokenHash]
	if !ok {
		return nil, apperrors.NotFound("identity.token_not_found", "no such refresh token")
	}
	cp := *rt
	return &cp, nil
}

func (f *fakeTokenRepo) MarkConsumed(_ context.Context, tokenID uuid.UUID, consumedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rt := range f.byHash {
		if rt.ID == tokenID {
			rt.ConsumedAt = &consumedAt
			return nil
		}
	}
	return apperrors.NotFound("identity.token_not_found", "no such refresh token")
}

func (f *fakeTokenRepo) RevokeFamily(_ context.Context, familyID uuid.UUID, revokedAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rt := range f.byHash {
		if rt.FamilyID == familyID {
			rt.RevokedAt = &revokedAt
		}
	}
	return nil
}

// backdateAllFamiliesIssuedAt is a test-only helper (no production Service
// method needs this — FamilyIssuedAt is only ever set at issuance) that
// simulates every currently-stored token's family having originally been
// issued at issuedAt, for the absolute-session-cap tests below. Safe to call
// with "every" rather than a specific family ID because both call sites use
// it right after a single Login, when exactly one row exists. A real
// clock-injection seam isn't worth adding just for this one check, given
// every other timestamp in this package already comes from time.Now()
// directly.
func (f *fakeTokenRepo) backdateAllFamiliesIssuedAt(issuedAt time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rt := range f.byHash {
		rt.FamilyIssuedAt = issuedAt
	}
}

// --- test harness ---

const testJWTSecret = "test-secret-at-least-32-bytes-long!!"

func newTestService(t *testing.T) (identity.Service, *fakeUserRepo, *fakeTokenRepo) {
	t.Helper()
	svc, users, tokens, _ := newTestServiceWithAudit(t)
	return svc, users, tokens
}

// newTestServiceWithAudit is newTestService plus the fakeAuditService, for
// the tests that assert on BUILD_GUIDE.md Phase 6's audit instrumentation
// directly.
func newTestServiceWithAudit(t *testing.T) (identity.Service, *fakeUserRepo, *fakeTokenRepo, *fakeAuditService) {
	t.Helper()
	orgID := id.New()
	users := newFakeUserRepo(orgID)
	orgs := &fakeOrgRepo{}
	tokens := newFakeTokenRepo()
	auditSvc := &fakeAuditService{}
	issuer := identity.NewTokenIssuer([]byte(testJWTSecret), 15*time.Minute)
	svc := identity.NewService(users, orgs, tokens, issuer, auditSvc, 15*time.Minute, 30*time.Minute, 12*time.Hour, nil, nil)
	return svc, users, tokens, auditSvc
}

func appErrCode(t *testing.T, err error) string {
	t.Helper()
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error %v is not an *apperrors.Error", err)
	}
	return appErr.Code
}

// --- Register ---

func TestRegister_NewUserBecomesAdminOfItsOwnOrganization(t *testing.T) {
	svc, _, _ := newTestService(t)

	user, err := svc.Register(context.Background(), identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia R.", Password: "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	// Every registrant is the admin of their own brand-new organisation —
	// there's no "first user overall" special case any more (see the
	// multi-tenancy fix in PROGRESS-LOG.md).
	if user.Role != domain.RoleAdmin {
		t.Errorf("registrant's role = %q, want %q", user.Role, domain.RoleAdmin)
	}
	if user.OrgID == uuid.Nil {
		t.Error("registrant has no organisation assigned")
	}
}

func TestRegister_TwoUsersGetSeparateIsolatedOrganizations(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	first, err := svc.Register(ctx, identity.RegisterInput{
		Email: "first@example.com", DisplayName: "First", Password: "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("first Register() error = %v", err)
	}

	second, err := svc.Register(ctx, identity.RegisterInput{
		Email: "second@example.com", DisplayName: "Second", Password: "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("second Register() error = %v", err)
	}

	// This is the regression test for the cross-account data leak: two
	// separate registrations must land in two separate organisations, not
	// share one — otherwise every project/scan/finding query scoped by
	// actor.OrgID (internal/modules/project/service.go) would show one
	// account's data to the other.
	if first.OrgID == second.OrgID {
		t.Fatalf("two independent registrations share an organisation (%v) — this is the cross-account leak bug, not expected behaviour", first.OrgID)
	}
	if second.Role != domain.RoleAdmin {
		t.Errorf("second user's role = %q, want %q (admin of their own new org, not a member of the first user's org)", second.Role, domain.RoleAdmin)
	}
}

func TestRegister_DuplicateEmailIsRejected(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	in := identity.RegisterInput{Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery"}

	if _, err := svc.Register(ctx, in); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	_, err := svc.Register(ctx, identity.RegisterInput{
		Email: "NADIA@example.com", DisplayName: "Nadia Again", Password: "correct-horse-battery",
	})
	if err == nil {
		t.Fatal("Register() error = nil, want a conflict for a duplicate email")
	}
	if got := appErrCode(t, err); got != "identity.email_taken" {
		t.Errorf("error code = %q, want %q", got, "identity.email_taken")
	}
}

func TestRegister_WeakPasswordIsRejected(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Register(context.Background(), identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "short",
	})
	if err == nil {
		t.Fatal("Register() error = nil, want a validation error for a short password")
	}
}

// TestRegister_TwelveCharacterPasswordIsAccepted is the near-miss boundary
// check for the previous test: the doc says "≥ 12 characters" — exactly 12
// must pass.
func TestRegister_TwelveCharacterPasswordIsAccepted(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Register(context.Background(), identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "xk8x7f2m4q9z",
	})
	if err != nil {
		t.Errorf("Register() error = %v, want nil for an exactly-12-character password", err)
	}
}

func TestRegister_CommonPasswordIsRejected(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Register(context.Background(), identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "administrator",
	})
	if err == nil {
		t.Fatal("Register() error = nil, want a validation error for a common password")
	}
}

// --- Login ---

func TestLogin_CorrectCredentialsIssueTokens(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	pair, err := svc.Login(ctx, "nadia@example.com", "correct-horse-battery")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Error("Login() returned an empty access or refresh token")
	}
	if pair.User == nil || pair.User.Email != "nadia@example.com" {
		t.Errorf("Login() user = %+v, want the logged-in user", pair.User)
	}
}

// TestLogin_RecordsAuditEntry is BUILD_GUIDE.md Phase 6's retroactive
// instrumentation requirement: a successful login must append to
// audit_log, not just issue tokens.
func TestLogin_RecordsAuditEntry(t *testing.T) {
	svc, _, _, auditSvc := newTestServiceWithAudit(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if _, err := svc.Login(ctx, "nadia@example.com", "correct-horse-battery"); err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	actions := auditSvc.actions()
	if len(actions) != 1 || actions[0] != "auth.login" {
		t.Errorf("audit actions = %v, want exactly [\"auth.login\"]", actions)
	}
}

// TestLogin_WrongPassword_RecordsNoAuditEntry is the near-miss: a failed
// login attempt is not a login and must not appear in the audit trail as
// one.
func TestLogin_WrongPassword_RecordsNoAuditEntry(t *testing.T) {
	svc, _, _, auditSvc := newTestServiceWithAudit(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if _, err := svc.Login(ctx, "nadia@example.com", "wrong-password"); err == nil {
		t.Fatal("Login() error = nil, want an error for a wrong password")
	}

	if actions := auditSvc.actions(); len(actions) != 0 {
		t.Errorf("audit actions = %v, want none for a failed login", actions)
	}
}

func TestLogin_WrongPasswordIsRejected(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	_, err := svc.Login(ctx, "nadia@example.com", "totally-wrong-password")
	if err == nil {
		t.Fatal("Login() error = nil, want an error for a wrong password")
	}
	if got := appErrCode(t, err); got != "auth.invalid_credentials" {
		t.Errorf("error code = %q, want %q", got, "auth.invalid_credentials")
	}
}

// TestLogin_UnknownEmailReturnsSameErrorAsWrongPassword is the
// no-user-enumeration requirement (documentation/07-api-specification.md
// §2, FR-IAM-009's spirit): an attacker must not be able to distinguish
// "no such account" from "wrong password" by response content.
func TestLogin_UnknownEmailReturnsSameErrorAsWrongPassword(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Login(context.Background(), "nobody@example.com", "whatever-password")
	if err == nil {
		t.Fatal("Login() error = nil, want an error for an unknown email")
	}
	if got := appErrCode(t, err); got != "auth.invalid_credentials" {
		t.Errorf("error code = %q, want the same %q code used for a wrong password (no enumeration)", got, "auth.invalid_credentials")
	}
}

func TestLogin_LocksAfterMaxFailedAttempts(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	for range 10 {
		_, _ = svc.Login(ctx, "nadia@example.com", "wrong-password")
	}

	// Now even the correct password must be rejected while locked.
	_, err := svc.Login(ctx, "nadia@example.com", "correct-horse-battery")
	if err == nil {
		t.Fatal("Login() with the correct password succeeded while the account should be locked")
	}
}

// --- Suspension (BUILD_GUIDE.md Phase 14) ---

// newTestServiceForSuspension is a dedicated helper (rather than extending
// newTestServiceWithAudit's return signature and breaking every existing
// call site) — these tests need direct access to the fakeOrgRepo, which
// none of the others do.
func newTestServiceForSuspension(t *testing.T) (identity.Service, *fakeUserRepo, *fakeOrgRepo) {
	t.Helper()
	orgID := id.New()
	users := newFakeUserRepo(orgID)
	orgs := &fakeOrgRepo{}
	tokens := newFakeTokenRepo()
	auditSvc := &fakeAuditService{}
	issuer := identity.NewTokenIssuer([]byte(testJWTSecret), 15*time.Minute)
	svc := identity.NewService(users, orgs, tokens, issuer, auditSvc, 15*time.Minute, 30*time.Minute, 12*time.Hour, nil, nil)
	return svc, users, orgs
}

func TestLogin_SuspendedUserIsRejected(t *testing.T) {
	svc, users, _ := newTestServiceForSuspension(t)
	ctx := context.Background()
	user, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	reason := "ToS violation"
	now := time.Now().UTC()
	users.mu.Lock()
	users.byID[user.ID].SuspendedAt, users.byID[user.ID].SuspendedReason = &now, &reason
	users.mu.Unlock()

	_, err = svc.Login(ctx, "nadia@example.com", "correct-horse-battery")
	if err == nil {
		t.Fatal("Login() error = nil, want a rejection for a suspended user")
	}
	if code := appErrCode(t, err); code != "auth.account_suspended" {
		t.Errorf("Login() error code = %q, want %q", code, "auth.account_suspended")
	}
}

func TestLogin_SuspendedOrganizationIsRejected(t *testing.T) {
	svc, _, orgs := newTestServiceForSuspension(t)
	ctx := context.Background()
	user, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	reason := "org suspended pending investigation"
	orgs.mu.Lock()
	if orgs.suspended == nil {
		orgs.suspended = map[uuid.UUID]string{}
	}
	orgs.suspended[user.OrgID] = reason
	orgs.mu.Unlock()

	_, err = svc.Login(ctx, "nadia@example.com", "correct-horse-battery")
	if err == nil {
		t.Fatal("Login() error = nil, want a rejection for a suspended organization")
	}
	if code := appErrCode(t, err); code != "auth.account_suspended" {
		t.Errorf("Login() error code = %q, want %q", code, "auth.account_suspended")
	}
}

// TestCheckSuspension_RejectsMidSessionSuspension is the near-miss that
// matters most for this feature: a user who logged in *before* being
// suspended must still be rejected on their very next request — this is
// what middleware.SuspensionCheck calls on every authenticated route, not
// just at Login.
func TestCheckSuspension_RejectsMidSessionSuspension(t *testing.T) {
	svc, users, _ := newTestServiceForSuspension(t)
	ctx := context.Background()
	user, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	actor := domain.Actor{UserID: user.ID, OrgID: user.OrgID, Role: user.Role}

	if err := svc.CheckSuspension(ctx, actor); err != nil {
		t.Fatalf("CheckSuspension() error = %v, want nil for an active account", err)
	}

	reason := "ToS violation"
	now := time.Now().UTC()
	users.mu.Lock()
	users.byID[user.ID].SuspendedAt, users.byID[user.ID].SuspendedReason = &now, &reason
	users.mu.Unlock()

	err = svc.CheckSuspension(ctx, actor)
	if err == nil {
		t.Fatal("CheckSuspension() error = nil, want a rejection after suspension")
	}
	if code := appErrCode(t, err); code != "auth.account_suspended" {
		t.Errorf("CheckSuspension() error code = %q, want %q", code, "auth.account_suspended")
	}
}

// --- Refresh ---

func TestRefresh_RotatesTheToken(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	loginPair, err := svc.Login(ctx, "nadia@example.com", "correct-horse-battery")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	refreshed, err := svc.Refresh(ctx, loginPair.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if refreshed.RefreshToken == loginPair.RefreshToken {
		t.Error("Refresh() returned the same refresh token instead of a rotated one")
	}
	if refreshed.AccessToken == "" {
		t.Error("Refresh() returned an empty access token")
	}
}

// TestRefresh_ReuseOfConsumedTokenRevokesTheFamily is the reuse-detection
// requirement (documentation/05-module-specifications.md §3): presenting an
// already-used refresh token must invalidate every token in its family, not
// just fail once.
func TestRefresh_ReuseOfConsumedTokenRevokesTheFamily(t *testing.T) {
	svc, _, _, auditSvc := newTestServiceWithAudit(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	loginPair, err := svc.Login(ctx, "nadia@example.com", "correct-horse-battery")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	rotated, err := svc.Refresh(ctx, loginPair.RefreshToken)
	if err != nil {
		t.Fatalf("first Refresh() error = %v", err)
	}

	// Reuse the original (now-consumed) token.
	_, err = svc.Refresh(ctx, loginPair.RefreshToken)
	if err == nil {
		t.Fatal("Refresh() with an already-consumed token succeeded, want an error")
	}
	if got := appErrCode(t, err); got != "auth.refresh_reused" {
		t.Errorf("error code = %q, want %q", got, "auth.refresh_reused")
	}

	// The rotated token (same family) must now be revoked too.
	_, err = svc.Refresh(ctx, rotated.RefreshToken)
	if err == nil {
		t.Fatal("Refresh() with the rotated token succeeded after family revocation, want an error")
	}

	// BUILD_GUIDE.md Phase 6: reuse detection is a security event and must
	// be audited, not just rejected.
	actions := auditSvc.actions()
	found := false
	for _, a := range actions {
		if a == "auth.refresh_reused" {
			found = true
		}
	}
	if !found {
		t.Errorf("audit actions = %v, want \"auth.refresh_reused\" present", actions)
	}
}

func TestRefresh_InvalidTokenIsRejected(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Refresh(context.Background(), "not-a-real-refresh-token")
	if err == nil {
		t.Fatal("Refresh() error = nil, want an error for an unknown refresh token")
	}
}

// TestRefresh_JustUnderAbsoluteSessionCapSucceeds is the near-miss half of
// the absolute-session-cap table (BUILD_GUIDE.md Phase 14,
// documentation/15-testing-strategy.md's "near-miss matters more" rule): a
// session that's been continuously active right up to just under the
// sessionAbsoluteTTL boundary (newTestServiceWithAudit uses 12h) must still
// refresh normally — the cap isn't allowed to be so aggressive it clips a
// legitimately still-active session a few seconds early.
func TestRefresh_JustUnderAbsoluteSessionCapSucceeds(t *testing.T) {
	svc, _, tokens, _ := newTestServiceWithAudit(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	loginPair, err := svc.Login(ctx, "nadia@example.com", "correct-horse-battery")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	tokens.backdateAllFamiliesIssuedAt(time.Now().UTC().Add(-(12*time.Hour - time.Minute)))

	if _, err := svc.Refresh(ctx, loginPair.RefreshToken); err != nil {
		t.Fatalf("Refresh() just under the absolute session cap: error = %v, want success", err)
	}
}

// TestRefresh_PastAbsoluteSessionCapIsRejected is the true-positive half:
// once a session's family is older than sessionAbsoluteTTL, Refresh must
// reject it — even though the individual token's own ExpiresAt (the idle
// timeout, reset forward on every prior rotation) hasn't been reached —
// and revoke the whole family so no later presented token in the same chain
// slips through either.
func TestRefresh_PastAbsoluteSessionCapIsRejected(t *testing.T) {
	svc, _, tokens, auditSvc := newTestServiceWithAudit(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	loginPair, err := svc.Login(ctx, "nadia@example.com", "correct-horse-battery")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	tokens.backdateAllFamiliesIssuedAt(time.Now().UTC().Add(-13 * time.Hour))

	_, err = svc.Refresh(ctx, loginPair.RefreshToken)
	if err == nil {
		t.Fatal("Refresh() past the absolute session cap succeeded, want an error")
	}
	if got := appErrCode(t, err); got != "auth.session_expired" {
		t.Errorf("error code = %q, want %q", got, "auth.session_expired")
	}

	actions := auditSvc.actions()
	found := false
	for _, a := range actions {
		if a == "auth.session_expired" {
			found = true
		}
	}
	if !found {
		t.Errorf("audit actions = %v, want \"auth.session_expired\" present", actions)
	}
}

// --- Logout ---

func TestLogout_RevokesTheSession(t *testing.T) {
	svc, _, _, auditSvc := newTestServiceWithAudit(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	pair, err := svc.Login(ctx, "nadia@example.com", "correct-horse-battery")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	if err := svc.Logout(ctx, pair.RefreshToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}

	_, err = svc.Refresh(ctx, pair.RefreshToken)
	if err == nil {
		t.Fatal("Refresh() succeeded after Logout(), want the session to be dead")
	}

	actions := auditSvc.actions()
	if len(actions) != 2 || actions[0] != "auth.login" || actions[1] != "auth.logout" {
		t.Errorf("audit actions = %v, want [\"auth.login\" \"auth.logout\"]", actions)
	}
}

func TestLogout_UnknownTokenIsIdempotent(t *testing.T) {
	svc, _, _ := newTestService(t)
	if err := svc.Logout(context.Background(), "never-issued-token"); err != nil {
		t.Errorf("Logout() error = %v, want nil (logout is idempotent)", err)
	}
}

// --- Verify / Me ---

func TestVerify_ValidAccessTokenRoundTrips(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	user, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	pair, err := svc.Login(ctx, "nadia@example.com", "correct-horse-battery")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	claims, err := svc.Verify(ctx, pair.AccessToken)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if claims.UserID != user.ID {
		t.Errorf("Verify().UserID = %v, want %v", claims.UserID, user.ID)
	}
	if claims.Role != domain.RoleAdmin {
		t.Errorf("Verify().Role = %q, want %q", claims.Role, domain.RoleAdmin)
	}
}

func TestVerify_ExpiredTokenReturnsTokenExpiredCode(t *testing.T) {
	issuer := identity.NewTokenIssuer([]byte(testJWTSecret), -1*time.Minute) // already expired
	svc := identity.NewService(newFakeUserRepo(id.New()), &fakeOrgRepo{}, newFakeTokenRepo(), issuer, &fakeAuditService{}, -1*time.Minute, time.Hour, 12*time.Hour, nil, nil)

	token, err := issuer.Issue(id.New(), id.New(), domain.RoleMember, nil)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	_, err = svc.Verify(context.Background(), token)
	if err == nil {
		t.Fatal("Verify() error = nil, want an error for an expired token")
	}
	if got := appErrCode(t, err); got != "auth.token_expired" {
		t.Errorf("error code = %q, want %q", got, "auth.token_expired")
	}
}

func TestVerify_MalformedTokenReturnsTokenInvalidCode(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Verify(context.Background(), "not-a-jwt-at-all")
	if err == nil {
		t.Fatal("Verify() error = nil, want an error for a malformed token")
	}
	if got := appErrCode(t, err); got != "auth.token_invalid" {
		t.Errorf("error code = %q, want %q", got, "auth.token_invalid")
	}
}

func TestMe_ReturnsTheActorsUser(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	user, err := svc.Register(ctx, identity.RegisterInput{
		Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, err := svc.Me(ctx, domain.Actor{UserID: user.ID, OrgID: user.OrgID, Role: user.Role})
	if err != nil {
		t.Fatalf("Me() error = %v", err)
	}
	if got.Email != "nadia@example.com" {
		t.Errorf("Me().Email = %q, want %q", got.Email, "nadia@example.com")
	}
}

func TestMe_UnknownActorReturnsNotFound(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.Me(context.Background(), domain.Actor{UserID: id.New()})
	if err == nil {
		t.Fatal("Me() error = nil, want an error for an unknown actor")
	}
}

// --- BUILD_GUIDE.md Phase 15 — switch-org / MembershipRoleReader ---

// fakeMembershipRoleReader is a hand-written fake for
// identity.MembershipRoleReader — no mocking framework.
type fakeMembershipRoleReader struct {
	mu    sync.Mutex
	roles map[string]domain.Role // key: orgID|userID
}

func newFakeMembershipRoleReader() *fakeMembershipRoleReader {
	return &fakeMembershipRoleReader{roles: map[string]domain.Role{}}
}

func (f *fakeMembershipRoleReader) set(orgID, userID uuid.UUID, role domain.Role) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.roles[orgID.String()+"|"+userID.String()] = role
}

func (f *fakeMembershipRoleReader) remove(orgID, userID uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.roles, orgID.String()+"|"+userID.String())
}

func (f *fakeMembershipRoleReader) GetRole(_ context.Context, orgID, userID uuid.UUID) (domain.Role, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	role, ok := f.roles[orgID.String()+"|"+userID.String()]
	if !ok {
		return "", apperrors.NotFound("organization.membership_not_found", "membership not found")
	}
	return role, nil
}

func newTestServiceWithMembershipRoles(t *testing.T, reader identity.MembershipRoleReader) (identity.Service, *fakeUserRepo, *fakeTokenRepo) {
	t.Helper()
	orgID := id.New()
	users := newFakeUserRepo(orgID)
	orgs := &fakeOrgRepo{}
	tokens := newFakeTokenRepo()
	issuer := identity.NewTokenIssuer([]byte(testJWTSecret), 15*time.Minute)
	svc := identity.NewService(users, orgs, tokens, issuer, &fakeAuditService{}, 15*time.Minute, 30*time.Minute, 12*time.Hour, reader, nil)
	return svc, users, tokens
}

func TestIssueTokenPairForOrg_MintsIndependentFamily(t *testing.T) {
	reader := newFakeMembershipRoleReader()
	svc, _, _ := newTestServiceWithMembershipRoles(t, reader)
	ctx := context.Background()

	user, err := svc.Register(ctx, identity.RegisterInput{Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	homePair, err := svc.Login(ctx, "nadia@example.com", "correct-horse-battery")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	otherOrgID := id.New()
	reader.set(otherOrgID, user.ID, domain.RoleViewer)

	switched, err := svc.IssueTokenPairForOrg(ctx, user.ID, otherOrgID, domain.RoleViewer)
	if err != nil {
		t.Fatalf("IssueTokenPairForOrg() error = %v", err)
	}
	if switched.RefreshToken == homePair.RefreshToken {
		t.Error("IssueTokenPairForOrg() reused the home session's refresh token instead of minting an independent family")
	}
	if switched.User == nil || switched.User.OrgID != otherOrgID || switched.User.Role != domain.RoleViewer {
		t.Errorf("IssueTokenPairForOrg() response user = %+v, want OrgID=%s Role=viewer", switched.User, otherOrgID)
	}

	// The original home session must still work, completely unaffected.
	if _, err := svc.Refresh(ctx, homePair.RefreshToken); err != nil {
		t.Errorf("home session Refresh() after switch-org error = %v, want nil (independent families)", err)
	}
}

// TestRefresh_SwitchedOrgSessionKeepsItsOrgContext is the correctness case
// the whole RefreshToken.OrgID column (migration 00022) exists for: without
// it, Refresh would rebuild the next access token from the caller's HOME
// org/role every time, silently reverting a switched session back to the
// wrong org on its very next refresh.
func TestRefresh_SwitchedOrgSessionKeepsItsOrgContext(t *testing.T) {
	reader := newFakeMembershipRoleReader()
	svc, _, _ := newTestServiceWithMembershipRoles(t, reader)
	ctx := context.Background()

	user, err := svc.Register(ctx, identity.RegisterInput{Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	otherOrgID := id.New()
	reader.set(otherOrgID, user.ID, domain.RoleMember)
	switched, err := svc.IssueTokenPairForOrg(ctx, user.ID, otherOrgID, domain.RoleMember)
	if err != nil {
		t.Fatalf("IssueTokenPairForOrg() error = %v", err)
	}

	refreshed, err := svc.Refresh(ctx, switched.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	claims, err := svc.Verify(ctx, refreshed.AccessToken)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if claims.OrgID != otherOrgID {
		t.Errorf("refreshed access token OrgID = %s, want %s (the switched org, not the home org)", claims.OrgID, otherOrgID)
	}
	if claims.Role != domain.RoleMember {
		t.Errorf("refreshed access token Role = %q, want %q", claims.Role, domain.RoleMember)
	}
}

// TestRefresh_RevokedMembershipRejectsFurtherRefresh covers being kicked
// from an org mid-session: the next refresh of a token pair scoped to that
// org must fail, and revoke the whole family, not just this one call.
func TestRefresh_RevokedMembershipRejectsFurtherRefresh(t *testing.T) {
	reader := newFakeMembershipRoleReader()
	svc, _, _ := newTestServiceWithMembershipRoles(t, reader)
	ctx := context.Background()

	user, err := svc.Register(ctx, identity.RegisterInput{Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	otherOrgID := id.New()
	reader.set(otherOrgID, user.ID, domain.RoleMember)
	switched, err := svc.IssueTokenPairForOrg(ctx, user.ID, otherOrgID, domain.RoleMember)
	if err != nil {
		t.Fatalf("IssueTokenPairForOrg() error = %v", err)
	}

	reader.remove(otherOrgID, user.ID) // the org's admin removed this member

	if _, err := svc.Refresh(ctx, switched.RefreshToken); err == nil {
		t.Fatal("Refresh() error = nil, want an error once the underlying membership is gone")
	}
}

// --- project-collaborators follow-up — switch-project / ProjectCollaboratorRoleReader ---

// fakeProjectCollaboratorRoleReader is a hand-written fake for
// identity.ProjectCollaboratorRoleReader — no mocking framework.
type fakeProjectCollaboratorRoleReader struct {
	mu   sync.Mutex
	rows map[string][2]any // key: projectID|userID -> [role, orgID]
}

func newFakeProjectCollaboratorRoleReader() *fakeProjectCollaboratorRoleReader {
	return &fakeProjectCollaboratorRoleReader{rows: map[string][2]any{}}
}

func (f *fakeProjectCollaboratorRoleReader) set(projectID, userID, orgID uuid.UUID, role domain.Role) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[projectID.String()+"|"+userID.String()] = [2]any{role, orgID}
}

func (f *fakeProjectCollaboratorRoleReader) remove(projectID, userID uuid.UUID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows, projectID.String()+"|"+userID.String())
}

func (f *fakeProjectCollaboratorRoleReader) GetRoleAndOrg(_ context.Context, projectID, userID uuid.UUID) (domain.Role, uuid.UUID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[projectID.String()+"|"+userID.String()]
	if !ok {
		return "", uuid.Nil, apperrors.NotFound("project.collaborator_not_found", "collaborator not found")
	}
	return row[0].(domain.Role), row[1].(uuid.UUID), nil
}

func newTestServiceWithProjectRoles(t *testing.T, reader identity.ProjectCollaboratorRoleReader) (identity.Service, *fakeUserRepo, *fakeTokenRepo) {
	t.Helper()
	orgID := id.New()
	users := newFakeUserRepo(orgID)
	orgs := &fakeOrgRepo{}
	tokens := newFakeTokenRepo()
	issuer := identity.NewTokenIssuer([]byte(testJWTSecret), 15*time.Minute)
	svc := identity.NewService(users, orgs, tokens, issuer, &fakeAuditService{}, 15*time.Minute, 30*time.Minute, 12*time.Hour, nil, reader)
	return svc, users, tokens
}

func TestIssueTokenPairForProject_ScopesTheTokenToOneProject(t *testing.T) {
	reader := newFakeProjectCollaboratorRoleReader()
	svc, _, _ := newTestServiceWithProjectRoles(t, reader)
	ctx := context.Background()

	user, err := svc.Register(ctx, identity.RegisterInput{Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	homePair, err := svc.Login(ctx, "nadia@example.com", "correct-horse-battery")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}

	sharedProjectOrgID := id.New()
	sharedProjectID := id.New()
	reader.set(sharedProjectID, user.ID, sharedProjectOrgID, domain.RoleMember)

	switched, err := svc.IssueTokenPairForProject(ctx, user.ID, sharedProjectID, sharedProjectOrgID, domain.RoleMember)
	if err != nil {
		t.Fatalf("IssueTokenPairForProject() error = %v", err)
	}
	if switched.RefreshToken == homePair.RefreshToken {
		t.Error("IssueTokenPairForProject() reused the home session's refresh token instead of minting an independent family")
	}

	claims, err := svc.Verify(ctx, switched.AccessToken)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if claims.OrgID != sharedProjectOrgID || claims.Role != domain.RoleMember {
		t.Errorf("claims = %+v, want OrgID=%s Role=member", claims, sharedProjectOrgID)
	}
	if claims.ProjectID == nil || *claims.ProjectID != sharedProjectID {
		t.Errorf("claims.ProjectID = %v, want %s", claims.ProjectID, sharedProjectID)
	}
	if switched.User == nil || switched.User.ScopedProjectID == nil || *switched.User.ScopedProjectID != sharedProjectID {
		t.Errorf("switched.User.ScopedProjectID = %v, want %s", switched.User, sharedProjectID)
	}

	// The original home session must still work, completely unaffected —
	// its own claims carry no project scope at all.
	homeClaims, err := svc.Verify(ctx, homePair.AccessToken)
	if err != nil {
		t.Fatalf("Verify(home) error = %v", err)
	}
	if homeClaims.ProjectID != nil {
		t.Errorf("home session claims.ProjectID = %v, want nil", homeClaims.ProjectID)
	}
}

func TestRefresh_SwitchedProjectSessionKeepsItsProjectScope(t *testing.T) {
	reader := newFakeProjectCollaboratorRoleReader()
	svc, _, _ := newTestServiceWithProjectRoles(t, reader)
	ctx := context.Background()

	user, err := svc.Register(ctx, identity.RegisterInput{Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	sharedProjectOrgID := id.New()
	sharedProjectID := id.New()
	reader.set(sharedProjectID, user.ID, sharedProjectOrgID, domain.RoleViewer)

	switched, err := svc.IssueTokenPairForProject(ctx, user.ID, sharedProjectID, sharedProjectOrgID, domain.RoleViewer)
	if err != nil {
		t.Fatalf("IssueTokenPairForProject() error = %v", err)
	}

	refreshed, err := svc.Refresh(ctx, switched.RefreshToken)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	claims, err := svc.Verify(ctx, refreshed.AccessToken)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if claims.ProjectID == nil || *claims.ProjectID != sharedProjectID {
		t.Errorf("refreshed claims.ProjectID = %v, want %s", claims.ProjectID, sharedProjectID)
	}
	if claims.OrgID != sharedProjectOrgID || claims.Role != domain.RoleViewer {
		t.Errorf("refreshed claims = %+v, want OrgID=%s Role=viewer", claims, sharedProjectOrgID)
	}
}

// TestRefresh_RevokedCollaboratorGrantRejectsFurtherRefresh mirrors
// TestRefresh_RevokedMembershipRejectsFurtherRefresh — a revoked project
// collaborator grant must kill the session on its very next refresh, the
// same way a revoked org membership already does.
func TestRefresh_RevokedCollaboratorGrantRejectsFurtherRefresh(t *testing.T) {
	reader := newFakeProjectCollaboratorRoleReader()
	svc, _, _ := newTestServiceWithProjectRoles(t, reader)
	ctx := context.Background()

	user, err := svc.Register(ctx, identity.RegisterInput{Email: "nadia@example.com", DisplayName: "Nadia", Password: "correct-horse-battery"})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	sharedProjectOrgID := id.New()
	sharedProjectID := id.New()
	reader.set(sharedProjectID, user.ID, sharedProjectOrgID, domain.RoleMember)

	switched, err := svc.IssueTokenPairForProject(ctx, user.ID, sharedProjectID, sharedProjectOrgID, domain.RoleMember)
	if err != nil {
		t.Fatalf("IssueTokenPairForProject() error = %v", err)
	}

	reader.remove(sharedProjectID, user.ID) // the project owner removed this collaborator

	if _, err := svc.Refresh(ctx, switched.RefreshToken); err == nil {
		t.Fatal("Refresh() error = nil, want an error once the underlying collaborator grant is gone")
	}
}
