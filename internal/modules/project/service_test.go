package project_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/vcs"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/crypto"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
)

// --- hand-written fakes (no mocking framework, matching identity's tests) ---

type fakeProjectRepo struct {
	mu   sync.Mutex
	byID map[uuid.UUID]*project.Project
}

func newFakeProjectRepo() *fakeProjectRepo {
	return &fakeProjectRepo{byID: map[uuid.UUID]*project.Project{}}
}

func (f *fakeProjectRepo) Create(_ context.Context, p *project.Project) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now().UTC()
	p.CreatedAt, p.UpdatedAt = now, now
	cp := *p
	f.byID[p.ID] = &cp
	return nil
}

func (f *fakeProjectRepo) GetByID(_ context.Context, id uuid.UUID) (*project.Project, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.byID[id]
	if !ok {
		return nil, apperrors.NotFound("project.not_found", "not found")
	}
	cp := *p
	return &cp, nil
}

func (f *fakeProjectRepo) GetByOrgAndName(_ context.Context, orgID uuid.UUID, name string) (*project.Project, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.byID {
		if p.OrgID == orgID && p.Name == name {
			cp := *p
			return &cp, nil
		}
	}
	return nil, apperrors.NotFound("project.not_found", "not found")
}

func (f *fakeProjectRepo) List(_ context.Context, orgID uuid.UUID, _ project.Page) ([]project.Project, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []project.Project
	for _, p := range f.byID {
		if p.OrgID == orgID {
			out = append(out, *p)
		}
	}
	return out, len(out), nil
}

func (f *fakeProjectRepo) Update(_ context.Context, p *project.Project) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byID[p.ID]; !ok {
		return apperrors.NotFound("project.not_found", "not found")
	}
	p.UpdatedAt = time.Now().UTC()
	cp := *p
	f.byID[p.ID] = &cp
	return nil
}

func (f *fakeProjectRepo) Delete(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.byID, id)
	return nil
}

type fakeRepositoryRepo struct {
	mu          sync.Mutex
	byProjectID map[uuid.UUID]*project.Repository
}

func newFakeRepositoryRepo() *fakeRepositoryRepo {
	return &fakeRepositoryRepo{byProjectID: map[uuid.UUID]*project.Repository{}}
}

func (f *fakeRepositoryRepo) Upsert(_ context.Context, r *project.Repository) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Mirrors the real RepositoryRepo.Upsert: an attach/replace always
	// clears a prior credential-invalid flag, regardless of what the
	// caller's struct happened to carry in.
	r.CredentialInvalidAt = nil
	r.CredentialInvalidReason = nil
	cp := *r
	f.byProjectID[r.ProjectID] = &cp
	return nil
}

func (f *fakeRepositoryRepo) GetByProjectID(_ context.Context, projectID uuid.UUID) (*project.Repository, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.byProjectID[projectID]
	if !ok {
		return nil, apperrors.NotFound("project.repository_not_found", "not found")
	}
	cp := *r
	return &cp, nil
}

func (f *fakeRepositoryRepo) MarkCredentialInvalid(_ context.Context, projectID uuid.UUID, reason string, at time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.byProjectID[projectID]
	if !ok {
		return apperrors.NotFound("project.repository_not_found", "not found")
	}
	r.CredentialInvalidAt = &at
	r.CredentialInvalidReason = &reason
	return nil
}

type credKey struct {
	projectID uuid.UUID
	kind      string
}

// fakeCredentialRepo performs real AES-256-GCM round trips (like the real
// repository would) so tests can assert the stored token actually decrypts
// back correctly, not just that Upsert was called.
type fakeCredentialRepo struct {
	mu   sync.Mutex
	rows map[credKey]project.CredentialRow
}

func newFakeCredentialRepo() *fakeCredentialRepo {
	return &fakeCredentialRepo{rows: map[credKey]project.CredentialRow{}}
}

func (f *fakeCredentialRepo) Upsert(_ context.Context, row project.CredentialRow) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[credKey{row.ProjectID, row.Kind}] = row
	return time.Now().UTC(), nil
}

func (f *fakeCredentialRepo) GetInfo(_ context.Context, projectID uuid.UUID, kind string) (*project.CredentialInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	row, ok := f.rows[credKey{projectID, kind}]
	if !ok {
		return nil, apperrors.NotFound("project.credential_not_found", "not found")
	}
	now := time.Now().UTC()
	return &project.CredentialInfo{HasCredential: true, Hint: row.Hint, UpdatedAt: &now}, nil
}

func (f *fakeCredentialRepo) GetPlaintext(_ context.Context, projectID uuid.UUID, kind string, key []byte) (string, error) {
	f.mu.Lock()
	row, ok := f.rows[credKey{projectID, kind}]
	f.mu.Unlock()
	if !ok {
		return "", apperrors.NotFound("project.credential_not_found", "not found")
	}
	plain, err := crypto.Decrypt(key, row.Ciphertext, row.Nonce)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (f *fakeCredentialRepo) Delete(_ context.Context, projectID uuid.UUID, kind string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.rows, credKey{projectID, kind})
	return nil
}

type fakeTargetRepo struct {
	mu   sync.Mutex
	byID map[uuid.UUID]*project.Target
}

func newFakeTargetRepo() *fakeTargetRepo {
	return &fakeTargetRepo{byID: map[uuid.UUID]*project.Target{}}
}

func (f *fakeTargetRepo) Create(_ context.Context, t *project.Target) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *t
	f.byID[t.ID] = &cp
	return nil
}

func (f *fakeTargetRepo) GetByID(_ context.Context, id uuid.UUID) (*project.Target, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	if !ok {
		return nil, apperrors.NotFound("target.not_found", "not found")
	}
	cp := *t
	return &cp, nil
}

func (f *fakeTargetRepo) ListByProject(_ context.Context, projectID uuid.UUID) ([]project.Target, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []project.Target
	for _, t := range f.byID {
		if t.ProjectID == projectID {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (f *fakeTargetRepo) UpdateStatus(_ context.Context, id uuid.UUID, status project.TargetStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.byID[id]
	if !ok {
		return apperrors.NotFound("target.not_found", "not found")
	}
	t.Status = status
	return nil
}

type fakeAttestationRepo struct {
	mu      sync.Mutex
	records []project.Attestation
}

func (f *fakeAttestationRepo) Create(_ context.Context, a *project.Attestation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, *a)
	return nil
}

type fakeDocumentRepo struct {
	mu   sync.Mutex
	byID map[uuid.UUID]*project.Document
}

func newFakeDocumentRepo() *fakeDocumentRepo {
	return &fakeDocumentRepo{byID: map[uuid.UUID]*project.Document{}}
}

func (f *fakeDocumentRepo) Create(_ context.Context, d *project.Document) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := *d
	f.byID[d.ID] = &cp
	return nil
}

func (f *fakeDocumentRepo) GetByID(_ context.Context, id uuid.UUID) (*project.Document, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.byID[id]
	if !ok {
		return nil, apperrors.NotFound("document.not_found", "not found")
	}
	cp := *d
	return &cp, nil
}

func (f *fakeDocumentRepo) ListByProject(_ context.Context, projectID uuid.UUID) ([]project.Document, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []project.Document
	for _, d := range f.byID {
		if d.ProjectID == projectID {
			out = append(out, *d)
		}
	}
	return out, nil
}

func (f *fakeDocumentRepo) CountByProject(_ context.Context, projectID uuid.UUID) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, d := range f.byID {
		if d.ProjectID == projectID {
			n++
		}
	}
	return n, nil
}

func (f *fakeDocumentRepo) Delete(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.byID[id]; !ok {
		return apperrors.NotFound("document.not_found", "not found")
	}
	delete(f.byID, id)
	return nil
}

type fakeUserLookup struct {
	name  string
	email string
}

func (f *fakeUserLookup) GetDisplayName(context.Context, uuid.UUID) (string, error) {
	return f.name, nil
}

func (f *fakeUserLookup) GetEmail(context.Context, uuid.UUID) (string, error) {
	return f.email, nil
}

// fakeOrgNameLookup is a hand-written fake for project.OrganizationNameLookup
// (project-collaborators follow-up) — no mocking framework.
type fakeOrgNameLookup struct{ name string }

func (f *fakeOrgNameLookup) GetName(context.Context, uuid.UUID) (string, error) {
	return f.name, nil
}

// fakeInviteRepo is a hand-written fake for project.ProjectInviteRepository
// (project-collaborators follow-up) — no mocking framework.
type fakeInviteRepo struct {
	mu      sync.Mutex
	byID    map[uuid.UUID]*project.ProjectInvite
	byToken map[string]*project.ProjectInvite
}

func newFakeInviteRepo() *fakeInviteRepo {
	return &fakeInviteRepo{byID: map[uuid.UUID]*project.ProjectInvite{}, byToken: map[string]*project.ProjectInvite{}}
}

func (f *fakeInviteRepo) Create(_ context.Context, in *project.ProjectInvite) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if in.ID == uuid.Nil {
		in.ID = id.New()
	}
	in.CreatedAt = time.Now().UTC()
	cp := *in
	f.byID[in.ID] = &cp
	f.byToken[in.TokenHash] = &cp
	return nil
}

func (f *fakeInviteRepo) GetByTokenHash(_ context.Context, tokenHash string) (*project.ProjectInvite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	inv, ok := f.byToken[tokenHash]
	if !ok {
		return nil, apperrors.NotFound("project.invite_not_found", "invite not found")
	}
	cp := *inv
	return &cp, nil
}

func (f *fakeInviteRepo) GetByID(_ context.Context, invID uuid.UUID) (*project.ProjectInvite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	inv, ok := f.byID[invID]
	if !ok {
		return nil, apperrors.NotFound("project.invite_not_found", "invite not found")
	}
	cp := *inv
	return &cp, nil
}

func (f *fakeInviteRepo) ListByProject(_ context.Context, projectID uuid.UUID) ([]project.ProjectInvite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []project.ProjectInvite
	for _, inv := range f.byID {
		if inv.ProjectID == projectID {
			out = append(out, *inv)
		}
	}
	return out, nil
}

func (f *fakeInviteRepo) ListByEmail(_ context.Context, email string) ([]project.ProjectInvite, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []project.ProjectInvite
	for _, inv := range f.byID {
		if inv.Email == email {
			out = append(out, *inv)
		}
	}
	return out, nil
}

func (f *fakeInviteRepo) ExistsPending(_ context.Context, projectID uuid.UUID, email string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, inv := range f.byID {
		if inv.ProjectID == projectID && inv.Email == email && inv.Status == project.InvitePending {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeInviteRepo) SetStatus(_ context.Context, invID uuid.UUID, status project.InviteStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	inv, ok := f.byID[invID]
	if !ok {
		return apperrors.NotFound("project.invite_not_found", "invite not found")
	}
	inv.Status = status
	return nil
}

// fakeCollaboratorRepo is a hand-written fake for
// project.ProjectCollaboratorRepository (project-collaborators follow-up) —
// no mocking framework.
type fakeCollaboratorRepo struct {
	mu   sync.Mutex
	rows map[[2]uuid.UUID]*project.ProjectCollaborator
}

func newFakeCollaboratorRepo() *fakeCollaboratorRepo {
	return &fakeCollaboratorRepo{rows: map[[2]uuid.UUID]*project.ProjectCollaborator{}}
}

func (f *fakeCollaboratorRepo) Create(_ context.Context, c *project.ProjectCollaborator) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c.ID == uuid.Nil {
		c.ID = id.New()
	}
	c.CreatedAt = time.Now().UTC()
	cp := *c
	f.rows[[2]uuid.UUID{c.ProjectID, c.UserID}] = &cp
	return nil
}

func (f *fakeCollaboratorRepo) GetByProjectAndUser(_ context.Context, projectID, userID uuid.UUID) (*project.ProjectCollaborator, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.rows[[2]uuid.UUID{projectID, userID}]
	if !ok {
		return nil, apperrors.NotFound("project.collaborator_not_found", "collaborator not found")
	}
	cp := *c
	return &cp, nil
}

func (f *fakeCollaboratorRepo) ListByProject(_ context.Context, projectID uuid.UUID) ([]project.ProjectCollaborator, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []project.ProjectCollaborator
	for k, c := range f.rows {
		if k[0] == projectID {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (f *fakeCollaboratorRepo) ListByUser(_ context.Context, userID uuid.UUID) ([]project.ProjectCollaborator, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []project.ProjectCollaborator
	for k, c := range f.rows {
		if k[1] == userID {
			out = append(out, *c)
		}
	}
	return out, nil
}

func (f *fakeCollaboratorRepo) Delete(_ context.Context, projectID, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := [2]uuid.UUID{projectID, userID}
	if _, ok := f.rows[key]; !ok {
		return apperrors.NotFound("project.collaborator_not_found", "collaborator not found")
	}
	delete(f.rows, key)
	return nil
}

type fakeVCS struct {
	info     *vcs.RepoInfo
	err      error
	cloneErr error
}

func (f *fakeVCS) ValidateRepository(context.Context, string, string) (*vcs.RepoInfo, error) {
	return f.info, f.err
}
func (f *fakeVCS) ShallowClone(context.Context, string, string, string, string) error {
	return f.cloneErr
}

type fakeResolver map[string][]net.IPAddr

func (f fakeResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	addrs, ok := f[host]
	if !ok {
		return nil, errors.New("no such host")
	}
	return addrs, nil
}

func ipAddrs(ips ...string) []net.IPAddr {
	out := make([]net.IPAddr, len(ips))
	for i, s := range ips {
		out[i] = net.IPAddr{IP: net.ParseIP(s)}
	}
	return out
}

// testDeps bundles everything needed to build a project.Service under test,
// with sensible defaults each test can override before calling build().
type testDeps struct {
	projects      *fakeProjectRepo
	repositories  *fakeRepositoryRepo
	credentials   *fakeCredentialRepo
	targets       *fakeTargetRepo
	attestations  *fakeAttestationRepo
	documents     *fakeDocumentRepo
	assignments   *fakeAssignmentRepo
	invites       *fakeInviteRepo
	collaborators *fakeCollaboratorRepo
	membership    *fakeMembershipChecker
	users         *fakeUserLookup
	orgs          *fakeOrgNameLookup
	identitySvc   identity.Service
	vcs           *fakeVCS
	resolver      fakeResolver
	audit         *fakeAuditService
	urlFetcher    *fakeURLFetcher
	pdfExtractor  *fakePDFExtractor
	denylist      []string
}

// fakeAssignmentRepo is a hand-written fake for
// project.ProjectAssignmentRepository (BUILD_GUIDE.md Phase 15) — no
// mocking framework.
type fakeAssignmentRepo struct {
	mu   sync.Mutex
	rows map[string]project.ProjectAssignment // key: projectID+"|"+userID
}

func newFakeAssignmentRepo() *fakeAssignmentRepo {
	return &fakeAssignmentRepo{rows: map[string]project.ProjectAssignment{}}
}

func assignmentKey(projectID, userID uuid.UUID) string {
	return projectID.String() + "|" + userID.String()
}

func (f *fakeAssignmentRepo) Create(_ context.Context, a *project.ProjectAssignment) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	a.AssignedAt = time.Now().UTC()
	f.rows[assignmentKey(a.ProjectID, a.UserID)] = *a
	return nil
}

func (f *fakeAssignmentRepo) Delete(_ context.Context, projectID, userID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := assignmentKey(projectID, userID)
	if _, ok := f.rows[k]; !ok {
		return apperrors.NotFound("project.assignment_not_found", "assignment not found")
	}
	delete(f.rows, k)
	return nil
}

func (f *fakeAssignmentRepo) ListByProject(_ context.Context, projectID uuid.UUID) ([]project.ProjectAssignment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []project.ProjectAssignment
	for _, a := range f.rows {
		if a.ProjectID == projectID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeAssignmentRepo) ListByOrg(_ context.Context, _ uuid.UUID) ([]project.ProjectAssignment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]project.ProjectAssignment, 0, len(f.rows))
	for _, a := range f.rows {
		out = append(out, a)
	}
	return out, nil
}

// fakeMembershipChecker is a hand-written fake for project.MembershipChecker
// — members defaults to "everyone is a member" (true) so existing tests that
// don't care about this check keep passing; individual tests override it.
type fakeMembershipChecker struct {
	isMember bool
	err      error
}

func (f *fakeMembershipChecker) IsOrgMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return f.isMember, f.err
}

// fakeAuditService is a hand-written fake — no mocking framework, matching
// the project's testing philosophy. Records every entry so tests can
// assert BUILD_GUIDE.md Phase 6's retroactive instrumentation actually
// fires.
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

func newTestDeps() *testDeps {
	return &testDeps{
		projects:      newFakeProjectRepo(),
		repositories:  newFakeRepositoryRepo(),
		credentials:   newFakeCredentialRepo(),
		targets:       newFakeTargetRepo(),
		attestations:  &fakeAttestationRepo{},
		documents:     newFakeDocumentRepo(),
		assignments:   newFakeAssignmentRepo(),
		invites:       newFakeInviteRepo(),
		collaborators: newFakeCollaboratorRepo(),
		membership:    &fakeMembershipChecker{isMember: true},
		users:         &fakeUserLookup{name: "Nadia R.", email: "nadia@example.com"},
		orgs:          &fakeOrgNameLookup{name: "Acme Org"},
		vcs:           &fakeVCS{},
		resolver:      fakeResolver{},
		audit:         &fakeAuditService{},
		urlFetcher:    &fakeURLFetcher{contentType: "text/plain", body: []byte("Imported document content.")},
		pdfExtractor:  &fakePDFExtractor{text: "Extracted PDF content."},
		denylist:      nil,
	}
}

func (d *testDeps) build() project.Service {
	key := make([]byte, crypto.KeySize)
	return project.NewService(
		d.projects, d.repositories, d.credentials, d.targets, d.attestations, d.documents,
		d.assignments, d.invites, d.collaborators, d.membership, d.users, d.orgs, d.identitySvc,
		d.vcs, d.resolver, d.audit, d.urlFetcher, d.pdfExtractor, key, false, d.denylist,
	)
}

// fakeURLFetcher is a hand-written fake for project.URLFetcher — no
// mocking framework, and no live network call to Google.
type fakeURLFetcher struct {
	contentType string
	body        []byte
	err         error
}

func (f *fakeURLFetcher) Fetch(context.Context, string) (string, []byte, error) {
	return f.contentType, f.body, f.err
}

// fakePDFExtractor is a hand-written fake for project.PDFTextExtractor —
// the real github.com/ledongthuc/pdf parser is exercised separately in
// docpdf_test.go against an actual generated PDF; service_test.go only
// needs to control what extraction returns.
type fakePDFExtractor struct {
	text string
	err  error
}

func (f *fakePDFExtractor) ExtractText([]byte) (string, error) {
	return f.text, f.err
}

func newActor() domain.Actor {
	return domain.Actor{UserID: id.New(), OrgID: id.New(), Role: domain.RoleMember}
}

func TestCreate_NoRepository(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)
	require.Equal(t, "Payments API", detail.Name)
	require.Nil(t, detail.Repository)
	require.False(t, detail.HasCredential)
	require.False(t, detail.HasAttestedTarget)
}

// TestCreate_RecordsAuditEntry is BUILD_GUIDE.md Phase 6's retroactive
// instrumentation requirement: creating a project must append to
// audit_log.
func TestCreate_RecordsAuditEntry(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	actions := d.audit.actions()
	require.Len(t, actions, 1)
	require.Equal(t, "project.created", actions[0])
	require.Equal(t, detail.ID, *d.audit.entries[0].ResourceID)
}

// TestCreate_WithRepository_RecordsBothAuditEntries confirms attaching a
// repository during Create fires its own "repository.attached" entry in
// addition to "project.created" — two audit-worthy things happened, not one.
func TestCreate_WithRepository_RecordsBothAuditEntries(t *testing.T) {
	d := newTestDeps()
	d.vcs.info = &vcs.RepoInfo{Owner: "acme", Name: "payments-api", NormalizedURL: "https://github.com/acme/payments-api", DefaultBranch: "main", IsPrivate: false, SizeKB: 100}
	svc := d.build()
	actor := newActor()

	url := "https://github.com/acme/payments-api"
	_, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API", RepositoryURL: &url})
	require.NoError(t, err)

	actions := d.audit.actions()
	require.Equal(t, []string{"project.created", "repository.attached"}, actions)
}

func TestCreate_WithPublicRepository(t *testing.T) {
	d := newTestDeps()
	d.vcs.info = &vcs.RepoInfo{Owner: "acme", Name: "payments-api", NormalizedURL: "https://github.com/acme/payments-api", DefaultBranch: "main", IsPrivate: false, SizeKB: 100}
	svc := d.build()
	actor := newActor()

	url := "https://github.com/acme/payments-api"
	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API", RepositoryURL: &url})
	require.NoError(t, err)
	require.NotNil(t, detail.Repository)
	require.Equal(t, "acme", detail.Repository.Owner)
}

func TestCreate_PrivateRepoWithoutCredential_ReturnsCredentialRequired(t *testing.T) {
	d := newTestDeps()
	d.vcs.err = apperrors.NotFound("github.repository_not_found", "repository not found, or private and inaccessible with the given credential")
	svc := d.build()
	actor := newActor()

	url := "https://github.com/acme/private-repo"
	_, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Private", RepositoryURL: &url})
	requireCode(t, err, apperrors.KindUnprocessable, "project.credential_required")

	// The compensating delete must have run — no orphan project left behind.
	_, getErr := d.projects.GetByOrgAndName(context.Background(), actor.OrgID, "Private")
	require.Error(t, getErr)
}

func TestCreate_DuplicateNameInOrg_Conflicts(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	_, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	_, err = svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	requireCode(t, err, apperrors.KindConflict, "project.name_taken")
}

func TestCreate_SameNameDifferentOrg_Allowed(t *testing.T) {
	// Near-miss: the uniqueness check must be scoped by org, not global.
	d := newTestDeps()
	svc := d.build()

	_, err := svc.Create(context.Background(), newActor(), project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)
	_, err = svc.Create(context.Background(), newActor(), project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)
}

func TestGet_ProjectFromAnotherOrg_ReturnsNotFoundNot403(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	owner := newActor()

	detail, err := svc.Create(context.Background(), owner, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	intruder := newActor() // different OrgID
	_, err = svc.Get(context.Background(), intruder, detail.ID)
	requireCode(t, err, apperrors.KindNotFound, "project.not_found")
}

func TestUpdate_PartialFieldsOnly(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	newDesc := "updated description"
	updated, err := svc.Update(context.Background(), actor, detail.ID, project.UpdateProjectInput{Description: &newDesc})
	require.NoError(t, err)
	require.Equal(t, "Payments API", updated.Name) // untouched
	require.Equal(t, "updated description", *updated.Description)
}

func TestArchive_SetsStatus(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	require.NoError(t, svc.Archive(context.Background(), actor, detail.ID))
	got, err := svc.Get(context.Background(), actor, detail.ID)
	require.NoError(t, err)
	require.Equal(t, project.StatusArchived, got.Status)

	actions := d.audit.actions()
	require.Equal(t, []string{"project.created", "project.archived"}, actions)
}

// --- project assignments — BUILD_GUIDE.md Phase 15 ---

func TestAssignProject_RejectsNonOrgMember(t *testing.T) {
	d := newTestDeps()
	d.membership = &fakeMembershipChecker{isMember: false}
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	_, err = svc.AssignProject(context.Background(), actor, detail.ID, uuid.New())
	require.Error(t, err)
}

func TestAssignProject_ThenUnassign(t *testing.T) {
	d := newTestDeps()
	d.membership = &fakeMembershipChecker{isMember: true}
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	assigneeID := uuid.New()
	a, err := svc.AssignProject(context.Background(), actor, detail.ID, assigneeID)
	require.NoError(t, err)
	require.Equal(t, assigneeID, a.UserID)

	list, err := svc.ListAssignments(context.Background(), actor, detail.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)

	require.NoError(t, svc.UnassignProject(context.Background(), actor, detail.ID, assigneeID))
	list, err = svc.ListAssignments(context.Background(), actor, detail.ID)
	require.NoError(t, err)
	require.Empty(t, list)
}

// --- project collaborators (project-collaborators follow-up) ---

func TestInviteCollaborator_AcceptGrantsAccessDoesNotTouchOrgMembership(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	owner := newActor()

	detail, err := svc.Create(context.Background(), owner, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	created, err := svc.InviteCollaborator(context.Background(), owner, detail.ID, project.InviteCollaboratorInput{Email: "collab@example.com", Role: domain.RoleMember})
	require.NoError(t, err)
	require.Equal(t, project.InvitePending, created.Status)
	require.NotEmpty(t, created.RawToken)

	// A second invite to the same email while one is still pending is a
	// conflict — the same "an invite is already pending" rule org invites
	// already enforce.
	_, err = svc.InviteCollaborator(context.Background(), owner, detail.ID, project.InviteCollaboratorInput{Email: "collab@example.com", Role: domain.RoleMember})
	require.Error(t, err)

	// The invitee — a completely different account, in a different org —
	// accepts by the invite's id (the live-notification flow).
	collabUserID := id.New()
	d.users.email = "collab@example.com" // callerEmail resolves through the same fake for every actor in this test
	collaborator, err := svc.AcceptCollaboratorInvite(context.Background(), domain.Actor{UserID: collabUserID, OrgID: id.New(), Role: domain.RoleViewer}, created.ID.String())
	require.NoError(t, err)
	require.Equal(t, detail.ID, collaborator.ProjectID)
	require.Equal(t, domain.RoleMember, collaborator.Role)

	// Accepting twice is a conflict, not a silent no-op.
	_, err = svc.AcceptCollaboratorInvite(context.Background(), domain.Actor{UserID: collabUserID, OrgID: id.New(), Role: domain.RoleViewer}, created.ID.String())
	require.Error(t, err)

	list, err := svc.ListCollaborators(context.Background(), owner, detail.ID)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, collabUserID, list[0].UserID)
}

func TestAcceptCollaboratorInvite_EmailMismatchRejected(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	owner := newActor()

	detail, err := svc.Create(context.Background(), owner, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)
	created, err := svc.InviteCollaborator(context.Background(), owner, detail.ID, project.InviteCollaboratorInput{Email: "collab@example.com", Role: domain.RoleViewer})
	require.NoError(t, err)

	d.users.email = "someone-else@example.com"
	_, err = svc.AcceptCollaboratorInvite(context.Background(), domain.Actor{UserID: id.New(), OrgID: id.New(), Role: domain.RoleViewer}, created.ID.String())
	require.Error(t, err)
}

// TestGetOwnedProject_ScopedActorConfinedToItsOwnProject is the security-
// critical case: a token scoped to one shared project (actor.ProjectID set)
// must read every *other* project in the same org as 404 — not just be
// denied write access to it — the same "confirming existence is itself a
// leak" rule the org-boundary check right above it already enforces. This
// is what actually makes "added to one project" not become "sees the whole
// org's dashboard."
func TestGetOwnedProject_ScopedActorConfinedToItsOwnProject(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	owner := newActor()

	sharedProject, err := svc.Create(context.Background(), owner, project.CreateProjectInput{Name: "Shared With Collaborator"})
	require.NoError(t, err)
	otherProject, err := svc.Create(context.Background(), owner, project.CreateProjectInput{Name: "Not Shared"})
	require.NoError(t, err)

	scopedActor := domain.Actor{UserID: id.New(), OrgID: owner.OrgID, Role: domain.RoleMember, ProjectID: &sharedProject.ID}

	// The scoped project itself is reachable.
	_, err = svc.Get(context.Background(), scopedActor, sharedProject.ID)
	require.NoError(t, err)

	// Any other project in the very same org reads as not-found.
	_, err = svc.Get(context.Background(), scopedActor, otherProject.ID)
	require.Error(t, err)
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, apperrors.KindNotFound, appErr.Kind)

	// List transparently narrows to just the one scoped project.
	list, total, err := svc.List(context.Background(), scopedActor, project.Page{Page: 1, PageSize: 25})
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, list, 1)
	require.Equal(t, sharedProject.ID, list[0].ID)
}

func TestScopedActor_CannotCreateProjectsOrReadTeamDashboard(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	scopedActor := domain.Actor{UserID: id.New(), OrgID: id.New(), Role: domain.RoleAdmin, ProjectID: uuidPtr(id.New())}

	_, err := svc.Create(context.Background(), scopedActor, project.CreateProjectInput{Name: "Should Not Be Allowed"})
	require.Error(t, err)

	_, err = svc.ListAssignmentsForOrg(context.Background(), scopedActor)
	require.Error(t, err)
}

func uuidPtr(id uuid.UUID) *uuid.UUID { return &id }

// TestGetCloneInfo_PublicRepository_NoCredential confirms
// modules/orchestrator's worker gets an empty token (not an error) for a
// public repository with nothing attached — the common case.
func TestGetCloneInfo_PublicRepository_NoCredential(t *testing.T) {
	d := newTestDeps()
	d.vcs.info = &vcs.RepoInfo{Owner: "acme", Name: "payments-api", NormalizedURL: "https://github.com/acme/payments-api", DefaultBranch: "main", IsPrivate: false, SizeKB: 100}
	svc := d.build()
	actor := newActor()

	url := "https://github.com/acme/payments-api"
	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API", RepositoryURL: &url})
	require.NoError(t, err)

	repoURL, branch, token, err := svc.GetCloneInfo(context.Background(), detail.ID)
	require.NoError(t, err)
	require.Equal(t, "https://github.com/acme/payments-api", repoURL)
	require.Equal(t, "main", branch)
	require.Empty(t, token)
}

func TestGetCloneInfo_PrivateRepository_ReturnsDecryptedToken(t *testing.T) {
	d := newTestDeps()
	d.vcs.info = &vcs.RepoInfo{Owner: "acme", Name: "private-api", NormalizedURL: "https://github.com/acme/private-api", DefaultBranch: "main", IsPrivate: true, SizeKB: 50}
	svc := d.build()
	actor := newActor()

	url := "https://github.com/acme/private-api"
	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Private API", RepositoryURL: &url})
	require.NoError(t, err)
	_, err = svc.SetCredential(context.Background(), actor, detail.ID, project.CredentialKindGitHubPAT, "ghp_realtoken1234567890")
	require.NoError(t, err)

	_, _, token, err := svc.GetCloneInfo(context.Background(), detail.ID)
	require.NoError(t, err)
	require.Equal(t, "ghp_realtoken1234567890", token)
}

func TestGetCloneInfo_NoRepositoryAttached_ReturnsNotFound(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "No Repo Yet"})
	require.NoError(t, err)

	_, _, _, err = svc.GetCloneInfo(context.Background(), detail.ID)
	require.Error(t, err)
}

func TestSetCredential_NeverExposesRawToken_ButRoundTripsCorrectly(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	info, err := svc.SetCredential(context.Background(), actor, detail.ID, project.CredentialKindGitHubPAT, "ghp_abcdefghijklmnop3f9a")
	require.NoError(t, err)
	require.True(t, info.HasCredential)
	require.Equal(t, "ghp_••••3f9a", info.Hint)
	require.NotContains(t, info.Hint, "abcdefghijklmnop")

	// The repository layer can still recover the real token for cloning —
	// this is the one place allowed to do so.
	plain, err := d.credentials.GetPlaintext(context.Background(), detail.ID, project.CredentialKindGitHubPAT, make([]byte, crypto.KeySize))
	require.NoError(t, err)
	require.Equal(t, "ghp_abcdefghijklmnop3f9a", plain)
}

// TestMarkCredentialInvalid_PersistsAndSurfacesOnProjectDetail is the
// true-positive half: once the orchestrator flags a credential invalid, that
// must survive independently of any particular scan — a later GET on the
// project (composeDetail) has to carry it, not just that one scan's job
// result.
func TestMarkCredentialInvalid_PersistsAndSurfacesOnProjectDetail(t *testing.T) {
	d := newTestDeps()
	d.vcs.info = &vcs.RepoInfo{Owner: "acme", Name: "private-api", NormalizedURL: "https://github.com/acme/private-api", DefaultBranch: "main", IsPrivate: true, SizeKB: 50}
	svc := d.build()
	actor := newActor()

	url := "https://github.com/acme/private-api"
	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Private API", RepositoryURL: &url})
	require.NoError(t, err)
	require.Nil(t, detail.Repository.CredentialInvalidAt)

	require.NoError(t, svc.MarkCredentialInvalid(context.Background(), detail.ID, "github_credential_rejected"))

	got, err := svc.Get(context.Background(), actor, detail.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Repository.CredentialInvalidAt)
	require.NotNil(t, got.Repository.CredentialInvalidReason)
	require.Equal(t, "github_credential_rejected", *got.Repository.CredentialInvalidReason)

	// The project itself, and the repository row, are still exactly there —
	// this must never delete or archive anything.
	require.Equal(t, project.StatusActive, got.Status)
}

// TestMarkCredentialInvalid_NoRepositoryAttached_IsNoop is the near-miss: a
// project with no repository at all has nothing to flag, so this must
// succeed quietly rather than surfacing an internal error the orchestrator
// worker would otherwise have to specially ignore.
func TestMarkCredentialInvalid_NoRepositoryAttached_IsNoop(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "No Repo Yet"})
	require.NoError(t, err)

	require.NoError(t, svc.MarkCredentialInvalid(context.Background(), detail.ID, "github_credential_rejected"))
}

// TestAttachRepository_ReplacingRepo_ClearsCredentialInvalid is the other
// true-positive half: reattaching (the "connect a new token" flow) is
// exactly the user action that should clear a previously-invalid
// credential — this is what lets the project pick back up in place rather
// than needing to be recreated.
func TestAttachRepository_ReplacingRepo_ClearsCredentialInvalid(t *testing.T) {
	d := newTestDeps()
	d.vcs.info = &vcs.RepoInfo{Owner: "acme", Name: "private-api", NormalizedURL: "https://github.com/acme/private-api", DefaultBranch: "main", IsPrivate: true, SizeKB: 50}
	svc := d.build()
	actor := newActor()

	url := "https://github.com/acme/private-api"
	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Private API", RepositoryURL: &url})
	require.NoError(t, err)
	require.NoError(t, svc.MarkCredentialInvalid(context.Background(), detail.ID, "github_credential_rejected"))

	invalid, err := svc.Get(context.Background(), actor, detail.ID)
	require.NoError(t, err)
	require.NotNil(t, invalid.Repository.CredentialInvalidAt)

	_, err = svc.SetCredential(context.Background(), actor, detail.ID, project.CredentialKindGitHubPAT, "ghp_freshrotatedtoken1234")
	require.NoError(t, err)
	_, err = svc.AttachRepository(context.Background(), actor, detail.ID, project.RepositoryInput{URL: url})
	require.NoError(t, err)

	healed, err := svc.Get(context.Background(), actor, detail.ID)
	require.NoError(t, err)
	require.Nil(t, healed.Repository.CredentialInvalidAt, "reattaching must clear the invalid flag")
	require.Nil(t, healed.Repository.CredentialInvalidReason)
}

func TestRegisterTarget_BlockedAddress(t *testing.T) {
	d := newTestDeps()
	d.resolver["internal.acme.example"] = ipAddrs("192.168.1.50")
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	_, err = svc.RegisterTarget(context.Background(), actor, detail.ID, project.TargetInput{Target: "internal.acme.example"})
	requireCode(t, err, apperrors.KindUnprocessable, "target.blocked_address")
}

// TestRegisterTarget_DenylistedHost is the near-miss complement to
// TestRegisterTarget_ThenAttest_FullFlow's true positive: a public,
// resolvable host is still rejected when it's explicitly denylisted, even
// though it isn't in any blocked private range.
func TestRegisterTarget_DenylistedHost(t *testing.T) {
	d := newTestDeps()
	d.resolver["evil.example"] = ipAddrs("203.0.113.20")
	d.denylist = []string{"evil.example"}
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	_, err = svc.RegisterTarget(context.Background(), actor, detail.ID, project.TargetInput{Target: "evil.example"})
	requireCode(t, err, apperrors.KindUnprocessable, "target.blocked_address")
}

func TestRegisterTarget_ThenAttest_FullFlow(t *testing.T) {
	d := newTestDeps()
	d.resolver["staging.acme.example"] = ipAddrs("203.0.113.10")
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	target, err := svc.RegisterTarget(context.Background(), actor, detail.ID, project.TargetInput{Target: "https://staging.acme.example"})
	require.NoError(t, err)
	require.Equal(t, project.TargetAwaitingAttestation, target.Status)

	updated, attestation, err := svc.AttestTarget(context.Background(), actor, target.ID, project.AttestationInput{
		AttestationTextVersion: "v1", Accepted: true, SourceIP: "203.0.113.99",
	})
	require.NoError(t, err)
	require.Equal(t, project.TargetAttested, updated.Status)
	require.Equal(t, "Nadia R.", attestation.AttestedByName)
	require.Len(t, d.attestations.records, 1)

	// BUILD_GUIDE.md Phase 6: attestation is a legal record and must be
	// audited, IP included since AttestationInput already carries it.
	actions := d.audit.actions()
	require.Contains(t, actions, "target.attested")
	last := d.audit.entries[len(d.audit.entries)-1]
	require.Equal(t, "target.attested", last.Action)
	require.NotNil(t, last.IP)
	require.Equal(t, "203.0.113.99", last.IP.String())
}

// TestGet_HasAttestedTarget is the true-positive complement to
// TestCreate_NoRepository's near-miss: once a target is registered and
// attested, ProjectDetail.HasAttestedTarget must flip true — this is what
// lets orchestrator.resolveEngines and the frontend's engine picker both
// know pentest is runnable without a second targets round trip.
func TestGet_HasAttestedTarget(t *testing.T) {
	d := newTestDeps()
	d.resolver["staging.acme.example"] = ipAddrs("203.0.113.10")
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	target, err := svc.RegisterTarget(context.Background(), actor, detail.ID, project.TargetInput{Target: "staging.acme.example"})
	require.NoError(t, err)

	reGet, err := svc.Get(context.Background(), actor, detail.ID)
	require.NoError(t, err)
	require.False(t, reGet.HasAttestedTarget, "registered but not yet attested must not count")

	_, _, err = svc.AttestTarget(context.Background(), actor, target.ID, project.AttestationInput{
		AttestationTextVersion: "v1", Accepted: true, SourceIP: "203.0.113.99",
	})
	require.NoError(t, err)

	reGet, err = svc.Get(context.Background(), actor, detail.ID)
	require.NoError(t, err)
	require.True(t, reGet.HasAttestedTarget)
}

func TestAttestTarget_RequiresAcceptance(t *testing.T) {
	d := newTestDeps()
	d.resolver["staging.acme.example"] = ipAddrs("203.0.113.10")
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)
	target, err := svc.RegisterTarget(context.Background(), actor, detail.ID, project.TargetInput{Target: "staging.acme.example"})
	require.NoError(t, err)

	_, _, err = svc.AttestTarget(context.Background(), actor, target.ID, project.AttestationInput{AttestationTextVersion: "v1", Accepted: false})
	requireCode(t, err, apperrors.KindValidation, "target.attestation_required")
}

func TestRevokeTarget_FromAnotherOrg_NotFound(t *testing.T) {
	d := newTestDeps()
	d.resolver["staging.acme.example"] = ipAddrs("203.0.113.10")
	svc := d.build()
	owner := newActor()

	detail, err := svc.Create(context.Background(), owner, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)
	target, err := svc.RegisterTarget(context.Background(), owner, detail.ID, project.TargetInput{Target: "staging.acme.example"})
	require.NoError(t, err)

	intruder := newActor()
	err = svc.RevokeTarget(context.Background(), intruder, target.ID)
	// Ownership is enforced via the target's parent project, so the leak
	// check surfaces as "project.not_found" — either way, 404 and not a
	// 403 that would confirm the target exists.
	requireCode(t, err, apperrors.KindNotFound, "project.not_found")
}

func TestUploadDocument_ThenList(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	doc, err := svc.UploadDocument(context.Background(), actor, detail.ID, "srs.md", "text/markdown", []byte("# SRS\n\nContent."))
	require.NoError(t, err)
	require.Equal(t, "srs.md", doc.Filename)
	require.Equal(t, 15, doc.SizeBytes)
	require.NotNil(t, doc.UploadedBy)
	require.Equal(t, actor.UserID, *doc.UploadedBy)

	docs, err := svc.ListDocuments(context.Background(), actor, detail.ID)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, doc.ID, docs[0].ID)

	require.Contains(t, d.audit.actions(), "document.uploaded")
}

func TestUploadDocument_RejectsUnsupportedExtension(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	_, err = svc.UploadDocument(context.Background(), actor, detail.ID, "srs.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", []byte("PK\x03\x04"))
	requireCode(t, err, apperrors.KindValidation, "document.unsupported_type")
}

// TestUploadDocument_AcceptsEveryAllowedExtension is the near-miss's
// complement — every extension the allowlist names must actually be
// accepted, not just an unsupported one rejected. .pdf goes through
// createDocument's extraction path too (fakePDFExtractor stands in for the
// real parser here — that's covered against a real PDF in docpdf_test.go).
func TestUploadDocument_AcceptsEveryAllowedExtension(t *testing.T) {
	for _, name := range []string{"srs.md", "notes.txt", "spec.adoc", "design.rst", "report.pdf", "data.csv"} {
		t.Run(name, func(t *testing.T) {
			d := newTestDeps()
			svc := d.build()
			actor := newActor()
			detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
			require.NoError(t, err)

			_, err = svc.UploadDocument(context.Background(), actor, detail.ID, name, "text/plain", []byte("content"))
			require.NoError(t, err)
		})
	}
}

func TestUploadDocument_PDF_StoresExtractedText(t *testing.T) {
	d := newTestDeps()
	d.pdfExtractor = &fakePDFExtractor{text: "  Architecture Decision Record 001  \n"}
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	doc, err := svc.UploadDocument(context.Background(), actor, detail.ID, "adr.pdf", "application/pdf", []byte("%PDF-1.4 fake raw bytes"))
	require.NoError(t, err)
	require.Equal(t, "text/plain", doc.MIMEType)
	require.Equal(t, "Architecture Decision Record 001", string(doc.Content))
}

func TestUploadDocument_PDF_RejectsWhenExtractionFails(t *testing.T) {
	d := newTestDeps()
	d.pdfExtractor = &fakePDFExtractor{err: errors.New("corrupt PDF")}
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	_, err = svc.UploadDocument(context.Background(), actor, detail.ID, "adr.pdf", "application/pdf", []byte("not really a pdf"))
	requireCode(t, err, apperrors.KindValidation, "document.invalid_pdf")
}

// TestUploadDocument_PDF_RejectsWhenNoTextExtracted covers a scanned/
// image-only PDF — extraction succeeds mechanically but yields nothing to
// review, which must be rejected rather than silently stored as an empty
// document that never surfaces a docreview finding.
func TestUploadDocument_PDF_RejectsWhenNoTextExtracted(t *testing.T) {
	d := newTestDeps()
	d.pdfExtractor = &fakePDFExtractor{text: "   \n  "}
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	_, err = svc.UploadDocument(context.Background(), actor, detail.ID, "scanned.pdf", "application/pdf", []byte("fake raw pdf bytes"))
	requireCode(t, err, apperrors.KindValidation, "document.invalid_pdf")
}

func TestUploadDocument_RejectsOversizedFile(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	oversized := make([]byte, 100*1024+1)
	_, err = svc.UploadDocument(context.Background(), actor, detail.ID, "big.md", "text/markdown", oversized)
	requireCode(t, err, apperrors.KindValidation, "document.too_large")
}

func TestUploadDocument_RejectsOnceProjectLimitReached(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	for i := range 20 {
		_, err := svc.UploadDocument(context.Background(), actor, detail.ID, fmt.Sprintf("doc-%d.md", i), "text/markdown", []byte("content"))
		require.NoError(t, err)
	}

	_, err = svc.UploadDocument(context.Background(), actor, detail.ID, "one-too-many.md", "text/markdown", []byte("content"))
	requireCode(t, err, apperrors.KindUnprocessable, "document.limit_reached")
}

func TestDeleteDocument_FromAnotherOrg_NotFound(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	owner := newActor()

	detail, err := svc.Create(context.Background(), owner, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)
	doc, err := svc.UploadDocument(context.Background(), owner, detail.ID, "srs.md", "text/markdown", []byte("content"))
	require.NoError(t, err)

	intruder := newActor()
	err = svc.DeleteDocument(context.Background(), intruder, doc.ID)
	// Ownership is enforced via the document's parent project, same
	// 404-not-403 shape as every other project-scoped resource.
	requireCode(t, err, apperrors.KindNotFound, "project.not_found")
}

func TestDeleteDocument_RemovesIt(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)
	doc, err := svc.UploadDocument(context.Background(), actor, detail.ID, "srs.md", "text/markdown", []byte("content"))
	require.NoError(t, err)

	require.NoError(t, svc.DeleteDocument(context.Background(), actor, doc.ID))

	docs, err := svc.ListDocuments(context.Background(), actor, detail.ID)
	require.NoError(t, err)
	require.Empty(t, docs)
}

func TestImportDocumentFromURL_Success(t *testing.T) {
	d := newTestDeps()
	d.urlFetcher = &fakeURLFetcher{contentType: "text/plain; charset=UTF-8", body: []byte("# Architecture\n\nImported content.")}
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	doc, err := svc.ImportDocumentFromURL(context.Background(), actor, detail.ID, "https://docs.google.com/document/d/1AbC-xyz_123/edit?usp=sharing")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(doc.Filename, "google-doc-"))
	require.True(t, strings.HasSuffix(doc.Filename, ".txt"))
	require.Equal(t, "text/plain", doc.MIMEType)
	require.Equal(t, len("# Architecture\n\nImported content."), doc.SizeBytes)

	docs, err := svc.ListDocuments(context.Background(), actor, detail.ID)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Contains(t, d.audit.actions(), "document.uploaded")
}

func TestImportDocumentFromURL_RejectsNonGoogleURL(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	_, err = svc.ImportDocumentFromURL(context.Background(), actor, detail.ID, "https://example.com/not-a-google-doc")
	requireCode(t, err, apperrors.KindValidation, "document.invalid_google_url")
}

func TestImportDocumentFromURL_RejectsEmptyURL(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	_, err = svc.ImportDocumentFromURL(context.Background(), actor, detail.ID, "   ")
	requireCode(t, err, apperrors.KindValidation, "document.invalid_input")
}

// TestImportDocumentFromURL_RejectsWhenNotPubliclyShared covers the one
// signal available without OAuth to tell "not shared" apart from "shared
// but empty": Google serves an HTML sign-in page instead of the plain-text
// export for a link that isn't "anyone with the link can view."
func TestImportDocumentFromURL_RejectsWhenNotPubliclyShared(t *testing.T) {
	d := newTestDeps()
	d.urlFetcher = &fakeURLFetcher{contentType: "text/html; charset=UTF-8", body: []byte("<html>sign in</html>")}
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	_, err = svc.ImportDocumentFromURL(context.Background(), actor, detail.ID, "https://docs.google.com/document/d/1AbC-xyz_123/edit")
	requireCode(t, err, apperrors.KindUnprocessable, "document.not_publicly_accessible")
}

func TestImportDocumentFromURL_RejectsFetchFailure(t *testing.T) {
	d := newTestDeps()
	d.urlFetcher = &fakeURLFetcher{err: errors.New("connection reset")}
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	_, err = svc.ImportDocumentFromURL(context.Background(), actor, detail.ID, "https://docs.google.com/document/d/1AbC-xyz_123/edit")
	requireCode(t, err, apperrors.KindUnprocessable, "document.import_failed")
}

func TestImportDocumentFromURL_RejectsOversizedFetch(t *testing.T) {
	d := newTestDeps()
	d.urlFetcher = &fakeURLFetcher{contentType: "text/plain", body: make([]byte, 100*1024+1)}
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	_, err = svc.ImportDocumentFromURL(context.Background(), actor, detail.ID, "https://docs.google.com/document/d/1AbC-xyz_123/edit")
	requireCode(t, err, apperrors.KindValidation, "document.too_large")
}

func TestImportDocumentFromURL_RejectsOnceProjectLimitReached(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	for i := range 20 {
		_, err := svc.UploadDocument(context.Background(), actor, detail.ID, fmt.Sprintf("doc-%d.md", i), "text/markdown", []byte("content"))
		require.NoError(t, err)
	}

	_, err = svc.ImportDocumentFromURL(context.Background(), actor, detail.ID, "https://docs.google.com/document/d/1AbC-xyz_123/edit")
	requireCode(t, err, apperrors.KindUnprocessable, "document.limit_reached")
}

func TestImportDocumentFromURL_FromAnotherOrg_NotFound(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	owner := newActor()

	detail, err := svc.Create(context.Background(), owner, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)

	intruder := newActor()
	_, err = svc.ImportDocumentFromURL(context.Background(), intruder, detail.ID, "https://docs.google.com/document/d/1AbC-xyz_123/edit")
	requireCode(t, err, apperrors.KindNotFound, "project.not_found")
}

// TestGetDocuments_NoActorCheck confirms the worker-facing read (no actor
// parameter — same reasoning as GetCloneInfo) returns a project's documents
// without requiring ownership context, since the caller is background job
// processing whose authorization already happened at scan creation.
func TestGetDocuments_NoActorCheck(t *testing.T) {
	d := newTestDeps()
	svc := d.build()
	actor := newActor()

	detail, err := svc.Create(context.Background(), actor, project.CreateProjectInput{Name: "Payments API"})
	require.NoError(t, err)
	_, err = svc.UploadDocument(context.Background(), actor, detail.ID, "srs.md", "text/markdown", []byte("content"))
	require.NoError(t, err)

	docs, err := svc.GetDocuments(context.Background(), detail.ID)
	require.NoError(t, err)
	require.Len(t, docs, 1)
}

func requireCode(t *testing.T, err error, wantKind apperrors.Kind, wantCode string) {
	t.Helper()
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, wantKind, appErr.Kind)
	require.Equal(t, wantCode, appErr.Code)
}
