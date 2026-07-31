package project_test

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
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

type fakeUserLookup struct{ name string }

func (f *fakeUserLookup) GetDisplayName(context.Context, uuid.UUID) (string, error) {
	return f.name, nil
}

type fakeVCS struct {
	info     *vcs.RepoInfo
	err      error
	cloneErr error
}

func (f *fakeVCS) ValidateRepository(context.Context, string, string) (*vcs.RepoInfo, error) {
	return f.info, f.err
}
func (f *fakeVCS) ShallowClone(context.Context, string, string, string) error { return f.cloneErr }

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
	projects     *fakeProjectRepo
	repositories *fakeRepositoryRepo
	credentials  *fakeCredentialRepo
	targets      *fakeTargetRepo
	attestations *fakeAttestationRepo
	users        *fakeUserLookup
	vcs          *fakeVCS
	resolver     fakeResolver
	allowlist    []string
}

func newTestDeps() *testDeps {
	return &testDeps{
		projects:     newFakeProjectRepo(),
		repositories: newFakeRepositoryRepo(),
		credentials:  newFakeCredentialRepo(),
		targets:      newFakeTargetRepo(),
		attestations: &fakeAttestationRepo{},
		users:        &fakeUserLookup{name: "Nadia R."},
		vcs:          &fakeVCS{},
		resolver:     fakeResolver{},
		allowlist:    []string{"acme.example"},
	}
}

func (d *testDeps) build() project.Service {
	key := make([]byte, crypto.KeySize)
	return project.NewService(d.projects, d.repositories, d.credentials, d.targets, d.attestations, d.users, d.vcs, d.resolver, key, false, d.allowlist)
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

func requireCode(t *testing.T, err error, wantKind apperrors.Kind, wantCode string) {
	t.Helper()
	var appErr *apperrors.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, wantKind, appErr.Kind)
	require.Equal(t, wantCode, appErr.Code)
}
