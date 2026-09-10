package organization_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/organization"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
)

// --- hand-written fakes (no mocking framework, matching identity's/project's tests) ---

type fakeMembershipRepo struct {
	rows map[string]organization.Membership // key: orgID|userID
}

func newFakeMembershipRepo() *fakeMembershipRepo {
	return &fakeMembershipRepo{rows: map[string]organization.Membership{}}
}

func mKey(orgID, userID uuid.UUID) string { return orgID.String() + "|" + userID.String() }

func (f *fakeMembershipRepo) Create(_ context.Context, m *organization.Membership) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	m.CreatedAt = time.Now().UTC()
	f.rows[mKey(m.OrgID, m.UserID)] = *m
	return nil
}

func (f *fakeMembershipRepo) GetByOrgAndUser(_ context.Context, orgID, userID uuid.UUID) (*organization.Membership, error) {
	m, ok := f.rows[mKey(orgID, userID)]
	if !ok {
		return nil, apperrors.NotFound("organization.membership_not_found", "membership not found")
	}
	return &m, nil
}

func (f *fakeMembershipRepo) ListByOrg(_ context.Context, orgID uuid.UUID) ([]organization.Membership, error) {
	var out []organization.Membership
	for _, m := range f.rows {
		if m.OrgID == orgID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeMembershipRepo) ListByUser(_ context.Context, userID uuid.UUID) ([]organization.Membership, error) {
	var out []organization.Membership
	for _, m := range f.rows {
		if m.UserID == userID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f *fakeMembershipRepo) UpdateRole(_ context.Context, orgID, userID uuid.UUID, role domain.Role) error {
	k := mKey(orgID, userID)
	m, ok := f.rows[k]
	if !ok {
		return apperrors.NotFound("organization.membership_not_found", "membership not found")
	}
	m.Role = role
	f.rows[k] = m
	return nil
}

func (f *fakeMembershipRepo) Delete(_ context.Context, orgID, userID uuid.UUID) error {
	k := mKey(orgID, userID)
	if _, ok := f.rows[k]; !ok {
		return apperrors.NotFound("organization.membership_not_found", "membership not found")
	}
	delete(f.rows, k)
	return nil
}

type fakeInviteRepo struct {
	byID   map[uuid.UUID]organization.Invite
	byHash map[string]uuid.UUID
}

func newFakeInviteRepo() *fakeInviteRepo {
	return &fakeInviteRepo{byID: map[uuid.UUID]organization.Invite{}, byHash: map[string]uuid.UUID{}}
}

func (f *fakeInviteRepo) Create(_ context.Context, in *organization.Invite) error {
	if in.ID == uuid.Nil {
		in.ID = uuid.New()
	}
	in.CreatedAt = time.Now().UTC()
	f.byID[in.ID] = *in
	f.byHash[in.TokenHash] = in.ID
	return nil
}

func (f *fakeInviteRepo) GetByTokenHash(_ context.Context, tokenHash string) (*organization.Invite, error) {
	id, ok := f.byHash[tokenHash]
	if !ok {
		return nil, apperrors.NotFound("organization.invite_not_found", "invite not found")
	}
	inv := f.byID[id]
	return &inv, nil
}

func (f *fakeInviteRepo) GetByID(_ context.Context, id uuid.UUID) (*organization.Invite, error) {
	inv, ok := f.byID[id]
	if !ok {
		return nil, apperrors.NotFound("organization.invite_not_found", "invite not found")
	}
	return &inv, nil
}

func (f *fakeInviteRepo) ListByOrg(_ context.Context, orgID uuid.UUID) ([]organization.Invite, error) {
	var out []organization.Invite
	for _, inv := range f.byID {
		if inv.OrgID == orgID {
			out = append(out, inv)
		}
	}
	return out, nil
}

func (f *fakeInviteRepo) ListByEmail(_ context.Context, email string) ([]organization.Invite, error) {
	var out []organization.Invite
	for _, inv := range f.byID {
		if inv.Email == email {
			out = append(out, inv)
		}
	}
	return out, nil
}

func (f *fakeInviteRepo) ExistsPending(_ context.Context, orgID uuid.UUID, email string) (bool, error) {
	for _, inv := range f.byID {
		if inv.OrgID == orgID && inv.Email == email && inv.Status == organization.InvitePending && inv.ExpiresAt.After(time.Now().UTC()) {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeInviteRepo) SetStatus(_ context.Context, id uuid.UUID, status organization.InviteStatus) error {
	inv, ok := f.byID[id]
	if !ok {
		return apperrors.NotFound("organization.invite_not_found", "invite not found")
	}
	inv.Status = status
	f.byID[id] = inv
	return nil
}

type fakeUserReader struct {
	byID map[uuid.UUID]organization.UserInfo
}

func newFakeUserReader(users ...organization.UserInfo) *fakeUserReader {
	f := &fakeUserReader{byID: map[uuid.UUID]organization.UserInfo{}}
	for _, u := range users {
		f.byID[u.ID] = u
	}
	return f
}

func (f *fakeUserReader) GetInfoByID(_ context.Context, id uuid.UUID) (*organization.UserInfo, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, apperrors.NotFound("organization.user_not_found", "user not found")
	}
	return &u, nil
}

func (f *fakeUserReader) ListHomeMembers(_ context.Context, orgID uuid.UUID) ([]organization.UserInfo, error) {
	var out []organization.UserInfo
	for _, u := range f.byID {
		if u.OrgID == orgID {
			out = append(out, u)
		}
	}
	return out, nil
}

func (f *fakeUserReader) SetHomeRole(_ context.Context, id uuid.UUID, role domain.Role) error {
	u, ok := f.byID[id]
	if !ok {
		return apperrors.NotFound("organization.user_not_found", "user not found")
	}
	u.Role = role
	f.byID[id] = u
	return nil
}

type fakeOrgReader struct {
	names map[uuid.UUID]string
}

func (f *fakeOrgReader) GetName(_ context.Context, orgID uuid.UUID) (string, error) {
	name, ok := f.names[orgID]
	if !ok {
		return "", apperrors.NotFound("organization.not_found", "organization not found")
	}
	return name, nil
}

// fakeIdentityService implements only what organization.Service needs from
// identity.Service (IssueTokenPairForOrg) — every other method panics if
// called, so a test that reaches one by accident fails loudly rather than
// silently returning a zero value.
type fakeIdentityService struct {
	identity.Service
	pair *identity.TokenPair
	err  error
}

func (f *fakeIdentityService) IssueTokenPairForOrg(context.Context, uuid.UUID, uuid.UUID, domain.Role) (*identity.TokenPair, error) {
	return f.pair, f.err
}

type fakeAuditService struct {
	entries []audit.Entry
}

func (f *fakeAuditService) Log(_ context.Context, e audit.Entry) { f.entries = append(f.entries, e) }
func (f *fakeAuditService) List(context.Context, audit.ListFilter, audit.Page) ([]audit.Entry, int, error) {
	return nil, 0, nil
}

// --- test scaffolding ---

type testDeps struct {
	memberships *fakeMembershipRepo
	invites     *fakeInviteRepo
	users       *fakeUserReader
	orgs        *fakeOrgReader
	identitySvc *fakeIdentityService
	audit       *fakeAuditService
}

func newTestDeps(users ...organization.UserInfo) *testDeps {
	orgNames := map[uuid.UUID]string{}
	for _, u := range users {
		orgNames[u.OrgID] = u.OrgID.String()
	}
	return &testDeps{
		memberships: newFakeMembershipRepo(),
		invites:     newFakeInviteRepo(),
		users:       newFakeUserReader(users...),
		orgs:        &fakeOrgReader{names: orgNames},
		identitySvc: &fakeIdentityService{},
		audit:       &fakeAuditService{},
	}
}

func (d *testDeps) build() organization.Service {
	return organization.NewService(d.memberships, d.invites, d.users, d.orgs, d.identitySvc, d.audit)
}

func newUser(orgID uuid.UUID, role domain.Role, email string) organization.UserInfo {
	return organization.UserInfo{ID: uuid.New(), OrgID: orgID, Email: email, DisplayName: email, Role: role}
}

// --- last-admin constraint (BUILD_GUIDE.md Phase 15's own stated test requirement) ---

func TestUpdateMemberRole_LastAdminCannotBeDemoted(t *testing.T) {
	orgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	svc := d.build()
	ctx := context.Background()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	// The sole admin (the founder, no other admin exists yet) cannot be
	// demoted — this is the "last owner" invariant biting immediately.
	err := svc.UpdateMemberRole(ctx, actor, orgID, founder.ID, domain.RoleMember)
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindValidation, appErr.Kind)
}

func TestUpdateMemberRole_DemoteAllowedWhenAnotherAdminExists(t *testing.T) {
	orgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	svc := d.build()
	ctx := context.Background()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	// A second admin joins via membership.
	invitedAdmin := uuid.New()
	require.NoError(t, d.memberships.Create(ctx, &organization.Membership{OrgID: orgID, UserID: invitedAdmin, Role: domain.RoleAdmin}))
	d.users.byID[invitedAdmin] = organization.UserInfo{ID: invitedAdmin, OrgID: uuid.New(), Email: "second@acme.test", DisplayName: "Second Admin", Role: domain.RoleAdmin}

	// Now the founder CAN be demoted, since another admin covers the floor.
	require.NoError(t, svc.UpdateMemberRole(ctx, actor, orgID, founder.ID, domain.RoleMember))

	// But now demoting/removing the last remaining admin (the invited one) is blocked.
	err := svc.UpdateMemberRole(ctx, actor, orgID, invitedAdmin, domain.RoleMember)
	require.Error(t, err)
}

func TestRemoveMember_LastAdminCannotBeRemoved(t *testing.T) {
	orgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	svc := d.build()
	ctx := context.Background()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	invitedAdmin := uuid.New()
	require.NoError(t, d.memberships.Create(ctx, &organization.Membership{OrgID: orgID, UserID: invitedAdmin, Role: domain.RoleAdmin}))
	d.users.byID[invitedAdmin] = organization.UserInfo{ID: invitedAdmin, OrgID: uuid.New(), Email: "second@acme.test", Role: domain.RoleAdmin}

	// The founder still covers the floor, so removing this one admin membership is fine...
	require.NoError(t, svc.RemoveMember(ctx, actor, orgID, invitedAdmin))

	// ...but the founder themselves can never be removed via this endpoint at all.
	err := svc.RemoveMember(ctx, actor, orgID, founder.ID)
	require.Error(t, err)
}

func TestRemoveMember_NonAdminAlwaysAllowed(t *testing.T) {
	orgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	svc := d.build()
	ctx := context.Background()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	memberID := uuid.New()
	require.NoError(t, d.memberships.Create(ctx, &organization.Membership{OrgID: orgID, UserID: memberID, Role: domain.RoleMember}))
	d.users.byID[memberID] = organization.UserInfo{ID: memberID, OrgID: uuid.New(), Email: "viewer@acme.test", Role: domain.RoleMember}

	require.NoError(t, svc.RemoveMember(ctx, actor, orgID, memberID))
	require.Equal(t, "organization.member_removed", d.audit.entries[len(d.audit.entries)-1].Action)
}

// --- cross-org "404 not 403" (documentation/07-api-specification.md §1.4's standing rule) ---

func TestListMembers_CrossOrgReturnsNotFound(t *testing.T) {
	orgID := uuid.New()
	otherOrgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	svc := d.build()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	_, err := svc.ListMembers(context.Background(), actor, otherOrgID)
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindNotFound, appErr.Kind)
}

// --- invites ---

func TestCreateInvite_ThenAccept(t *testing.T) {
	orgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	svc := d.build()
	ctx := context.Background()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	created, err := svc.CreateInvite(ctx, actor, orgID, organization.InviteInput{Email: "New.Hire@Acme.test", Role: domain.RoleMember})
	require.NoError(t, err)
	require.NotEmpty(t, created.RawToken)
	require.Equal(t, organization.InvitePending, created.Status)

	// The invited email now registers/logs in under its own home org and accepts.
	newHire := newUser(uuid.New(), domain.RoleAdmin, "new.hire@acme.test") // normalized-email match, case-insensitive
	d.users.byID[newHire.ID] = newHire
	accepterActor := domain.Actor{UserID: newHire.ID, OrgID: newHire.OrgID, Role: domain.RoleAdmin}

	membership, err := svc.AcceptInvite(ctx, accepterActor, created.RawToken)
	require.NoError(t, err)
	require.Equal(t, orgID, membership.OrgID)
	require.Equal(t, domain.RoleMember, membership.Role)

	// The invite is now consumed — accepting it again fails.
	_, err = svc.AcceptInvite(ctx, accepterActor, created.RawToken)
	require.Error(t, err)
}

func TestAcceptInvite_EmailMismatchRejected(t *testing.T) {
	orgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	svc := d.build()
	ctx := context.Background()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	created, err := svc.CreateInvite(ctx, actor, orgID, organization.InviteInput{Email: "intended@acme.test", Role: domain.RoleMember})
	require.NoError(t, err)

	wrongPerson := newUser(uuid.New(), domain.RoleAdmin, "someone-else@acme.test")
	d.users.byID[wrongPerson.ID] = wrongPerson
	wrongActor := domain.Actor{UserID: wrongPerson.ID, OrgID: wrongPerson.OrgID, Role: domain.RoleAdmin}

	_, err = svc.AcceptInvite(ctx, wrongActor, created.RawToken)
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindForbidden, appErr.Kind)
}

func TestAcceptInvite_ExpiredRejected(t *testing.T) {
	orgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	svc := d.build()
	ctx := context.Background()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	created, err := svc.CreateInvite(ctx, actor, orgID, organization.InviteInput{Email: "invitee@acme.test", Role: domain.RoleMember})
	require.NoError(t, err)

	// Backdate the invite past its TTL directly through the fake repo (the
	// raw token, and therefore its hash, is unaffected — only expires_at
	// changes), to exercise the expiry branch without waiting a real week.
	inv := d.invites.byID[created.ID]
	inv.ExpiresAt = time.Now().UTC().Add(-time.Hour)
	d.invites.byID[created.ID] = inv

	invitee := newUser(uuid.New(), domain.RoleAdmin, "invitee@acme.test")
	d.users.byID[invitee.ID] = invitee
	accepterActor := domain.Actor{UserID: invitee.ID, OrgID: invitee.OrgID, Role: domain.RoleAdmin}

	_, err = svc.AcceptInvite(ctx, accepterActor, created.RawToken)
	require.Error(t, err)
	require.Equal(t, organization.InviteExpired, d.invites.byID[created.ID].Status)
}

func TestSwitchOrg_NotAMemberRejected(t *testing.T) {
	orgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	svc := d.build()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	_, err := svc.SwitchOrg(context.Background(), actor, uuid.New())
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindForbidden, appErr.Kind)
}

func TestSwitchOrg_ToHomeOrgSucceeds(t *testing.T) {
	orgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	d.identitySvc.pair = &identity.TokenPair{AccessToken: "tok", RefreshToken: "rt", ExpiresIn: 900}
	svc := d.build()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	result, err := svc.SwitchOrg(context.Background(), actor, orgID)
	require.NoError(t, err)
	require.Equal(t, "tok", result.AccessToken)
}

func TestSwitchOrg_ToMemberOrgSucceeds(t *testing.T) {
	orgID := uuid.New()
	otherOrgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	require.NoError(t, d.memberships.Create(context.Background(), &organization.Membership{OrgID: otherOrgID, UserID: founder.ID, Role: domain.RoleViewer}))
	d.identitySvc.pair = &identity.TokenPair{AccessToken: "tok2", RefreshToken: "rt2", ExpiresIn: 900}
	svc := d.build()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	result, err := svc.SwitchOrg(context.Background(), actor, otherOrgID)
	require.NoError(t, err)
	require.Equal(t, "tok2", result.AccessToken)
}

// --- live notification feed (2026-09-02 follow-up) ---

func TestListMyInvites_ReturnsOnlyPendingUnexpiredForCallersEmail(t *testing.T) {
	orgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	svc := d.build()
	ctx := context.Background()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	invitee := newUser(uuid.New(), domain.RoleAdmin, "invitee@acme.test")
	d.users.byID[invitee.ID] = invitee
	inviteeActor := domain.Actor{UserID: invitee.ID, OrgID: invitee.OrgID, Role: domain.RoleAdmin}

	created, err := svc.CreateInvite(ctx, actor, orgID, organization.InviteInput{Email: "invitee@acme.test", Role: domain.RoleMember})
	require.NoError(t, err)

	// A second, already-expired invite for the same email must not show up.
	expiredOrg := uuid.New()
	expired := &organization.Invite{
		OrgID: expiredOrg, Email: "invitee@acme.test", Role: domain.RoleMember,
		TokenHash: "expiredhash", Status: organization.InvitePending, ExpiresAt: time.Now().UTC().Add(-time.Hour),
	}
	require.NoError(t, d.invites.Create(ctx, expired))
	d.orgs.names[expiredOrg] = "Expired Co"

	// A third invite for a DIFFERENT email must never show up.
	require.NoError(t, d.invites.Create(ctx, &organization.Invite{
		OrgID: orgID, Email: "someone-else@acme.test", Role: domain.RoleMember,
		TokenHash: "otherhash", Status: organization.InvitePending, ExpiresAt: time.Now().UTC().Add(time.Hour),
	}))

	list, err := svc.ListMyInvites(ctx, inviteeActor)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, created.ID, list[0].ID)
	require.Equal(t, orgID.String(), list[0].OrgID.String())

	// The expired one was normalized to InviteExpired as a side effect.
	got, err := d.invites.GetByID(ctx, expired.ID)
	require.NoError(t, err)
	require.Equal(t, organization.InviteExpired, got.Status)
}

func TestAcceptInvite_ByID_Succeeds(t *testing.T) {
	orgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	svc := d.build()
	ctx := context.Background()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	created, err := svc.CreateInvite(ctx, actor, orgID, organization.InviteInput{Email: "invitee@acme.test", Role: domain.RoleMember})
	require.NoError(t, err)

	invitee := newUser(uuid.New(), domain.RoleAdmin, "invitee@acme.test")
	d.users.byID[invitee.ID] = invitee
	inviteeActor := domain.Actor{UserID: invitee.ID, OrgID: invitee.OrgID, Role: domain.RoleAdmin}

	// Accepting by the invite's UUID (the live-notification flow) — no raw
	// token involved at all, unlike TestCreateInvite_ThenAccept.
	membership, err := svc.AcceptInvite(ctx, inviteeActor, created.ID.String())
	require.NoError(t, err)
	require.Equal(t, orgID, membership.OrgID)
}

func TestDeclineInvite_MarksDeclinedNotRevoked(t *testing.T) {
	orgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	svc := d.build()
	ctx := context.Background()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	created, err := svc.CreateInvite(ctx, actor, orgID, organization.InviteInput{Email: "invitee@acme.test", Role: domain.RoleMember})
	require.NoError(t, err)

	invitee := newUser(uuid.New(), domain.RoleAdmin, "invitee@acme.test")
	d.users.byID[invitee.ID] = invitee
	inviteeActor := domain.Actor{UserID: invitee.ID, OrgID: invitee.OrgID, Role: domain.RoleAdmin}

	require.NoError(t, svc.DeclineInvite(ctx, inviteeActor, created.ID))

	got, err := d.invites.GetByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, organization.InviteDeclined, got.Status)

	// Declined, not accepted — no membership was created, and the
	// notification feed no longer shows it.
	list, err := svc.ListMyInvites(ctx, inviteeActor)
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestDeclineInvite_WrongEmailRejected(t *testing.T) {
	orgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	svc := d.build()
	ctx := context.Background()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	created, err := svc.CreateInvite(ctx, actor, orgID, organization.InviteInput{Email: "intended@acme.test", Role: domain.RoleMember})
	require.NoError(t, err)

	wrongPerson := newUser(uuid.New(), domain.RoleAdmin, "someone-else@acme.test")
	d.users.byID[wrongPerson.ID] = wrongPerson
	wrongActor := domain.Actor{UserID: wrongPerson.ID, OrgID: wrongPerson.OrgID, Role: domain.RoleAdmin}

	err = svc.DeclineInvite(ctx, wrongActor, created.ID)
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindForbidden, appErr.Kind)
}

func TestSwitchOrg_IdentityErrorPropagates(t *testing.T) {
	orgID := uuid.New()
	founder := newUser(orgID, domain.RoleAdmin, "founder@acme.test")
	d := newTestDeps(founder)
	d.identitySvc.err = errors.New("boom")
	svc := d.build()
	actor := domain.Actor{UserID: founder.ID, OrgID: orgID, Role: domain.RoleAdmin}

	_, err := svc.SwitchOrg(context.Background(), actor, orgID)
	require.Error(t, err)
}
