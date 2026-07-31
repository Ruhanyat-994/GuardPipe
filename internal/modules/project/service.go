package project

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/github"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/vcs"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/crypto"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
)

// Service is projects, repositories, and pentest targets
// (documentation/05-module-specifications.md §4).
type Service interface {
	Create(ctx context.Context, actor domain.Actor, in CreateProjectInput) (*ProjectDetail, error)
	List(ctx context.Context, actor domain.Actor, page Page) ([]ProjectDetail, int, error)
	Get(ctx context.Context, actor domain.Actor, id uuid.UUID) (*ProjectDetail, error)
	Update(ctx context.Context, actor domain.Actor, id uuid.UUID, in UpdateProjectInput) (*ProjectDetail, error)
	Archive(ctx context.Context, actor domain.Actor, id uuid.UUID) error

	AttachRepository(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in RepositoryInput) (*Repository, error)
	SetCredential(ctx context.Context, actor domain.Actor, projectID uuid.UUID, kind, token string) (*CredentialInfo, error)
	RemoveCredential(ctx context.Context, actor domain.Actor, projectID uuid.UUID) error

	ListTargets(ctx context.Context, actor domain.Actor, projectID uuid.UUID) ([]Target, error)
	RegisterTarget(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in TargetInput) (*Target, error)
	AttestTarget(ctx context.Context, actor domain.Actor, targetID uuid.UUID, in AttestationInput) (*Target, *TargetAttestation, error)
	RevokeTarget(ctx context.Context, actor domain.Actor, targetID uuid.UUID) error
}

// ProjectDetail is a Project plus the pieces the API returns alongside it
// (documentation/07-api-specification.md §3's `POST /projects` example
// response) — no repository attached is a nil Repository, not an error.
type ProjectDetail struct {
	Project
	Repository     *Repository
	HasCredential  bool
	CredentialHint string
}

// ProjectRepository is defined by this package; implementation lives in
// internal/store/repo (documentation/04-backend-architecture.md §5.1).
type ProjectRepository interface {
	Create(ctx context.Context, p *Project) error
	GetByID(ctx context.Context, id uuid.UUID) (*Project, error)
	GetByOrgAndName(ctx context.Context, orgID uuid.UUID, name string) (*Project, error)
	List(ctx context.Context, orgID uuid.UUID, page Page) ([]Project, int, error)
	Update(ctx context.Context, p *Project) error
	// Delete is a hard delete, used only to compensate a repository-attach
	// failure right after Create — projects are otherwise soft-deleted via
	// Archive (Status = archived), never hard-deleted (FR-PRJ-008: full
	// scan history is retained).
	Delete(ctx context.Context, id uuid.UUID) error
}

// RepositoryRepository is defined by this package.
type RepositoryRepository interface {
	// Upsert inserts or replaces the one repository a project may have
	// (`repositories.project_id` is UNIQUE — documentation/06-database-design.md
	// §4.5).
	Upsert(ctx context.Context, r *Repository) error
	GetByProjectID(ctx context.Context, projectID uuid.UUID) (*Repository, error)
}

// CredentialRow is what CredentialRepository.Upsert writes — the encrypted
// form, never the plaintext token.
type CredentialRow struct {
	ProjectID  uuid.UUID
	Kind       string
	Ciphertext []byte
	Nonce      []byte
	Hint       string
	CreatedBy  uuid.UUID
}

// CredentialRepository is defined by this package. No caller outside this
// package's implementation may SELECT ciphertext
// (documentation/06-database-design.md §4.6's review-checklist rule) — that
// is why decryption (GetPlaintext) is a method here rather than the service
// reading ciphertext/nonce itself.
type CredentialRepository interface {
	Upsert(ctx context.Context, row CredentialRow) (updatedAt time.Time, err error)
	GetInfo(ctx context.Context, projectID uuid.UUID, kind string) (*CredentialInfo, error)
	GetPlaintext(ctx context.Context, projectID uuid.UUID, kind string, key []byte) (string, error)
	Delete(ctx context.Context, projectID uuid.UUID, kind string) error
}

// TargetRepository is defined by this package.
type TargetRepository interface {
	Create(ctx context.Context, t *Target) error
	GetByID(ctx context.Context, id uuid.UUID) (*Target, error)
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]Target, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status TargetStatus) error
}

// AttestationRepository is defined by this package.
type AttestationRepository interface {
	Create(ctx context.Context, a *Attestation) error
}

// UserDisplayNameLookup is the one thing this module needs from `identity`
// — the attesting user's display name for the attestation response
// (documentation/07-api-specification.md §4's "attested_by"). The module
// dependency table (documentation/05-module-specifications.md §1) lists
// `project → identity, vcs`, so this dependency is intentional, not a
// layering violation.
type UserDisplayNameLookup interface {
	GetDisplayName(ctx context.Context, userID uuid.UUID) (string, error)
}

type service struct {
	projects      ProjectRepository
	repositories  RepositoryRepository
	credentials   CredentialRepository
	targets       TargetRepository
	attestations  AttestationRepository
	users         UserDisplayNameLookup
	vcs           vcs.Service
	resolver      validate.Resolver
	encryptionKey []byte

	allowPrivateTargets bool
	pentestAllowlist    []string
	attestationVersion  string
}

// NewService wires the project module.
func NewService(
	projects ProjectRepository,
	repositories RepositoryRepository,
	credentials CredentialRepository,
	targets TargetRepository,
	attestations AttestationRepository,
	users UserDisplayNameLookup,
	vcsSvc vcs.Service,
	resolver validate.Resolver,
	encryptionKey []byte,
	allowPrivateTargets bool,
	pentestAllowlist []string,
) Service {
	return &service{
		projects:            projects,
		repositories:        repositories,
		credentials:         credentials,
		targets:             targets,
		attestations:        attestations,
		users:               users,
		vcs:                 vcsSvc,
		resolver:            resolver,
		encryptionKey:       encryptionKey,
		allowPrivateTargets: allowPrivateTargets,
		pentestAllowlist:    pentestAllowlist,
		attestationVersion:  "v1",
	}
}

func (s *service) Create(ctx context.Context, actor domain.Actor, in CreateProjectInput) (*ProjectDetail, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 120 {
		return nil, apperrors.Validation("project.invalid_input", "name must be between 1 and 120 characters", nil)
	}
	description := trimmedOrNil(in.Description)

	if _, err := s.projects.GetByOrgAndName(ctx, actor.OrgID, name); err == nil {
		return nil, apperrors.Conflict("project.name_taken", "a project with this name already exists")
	} else if !isNotFound(err) {
		return nil, apperrors.Internal(fmt.Errorf("check existing project name: %w", err))
	}

	createdBy := actor.UserID
	p := &Project{
		ID:          id.New(),
		OrgID:       actor.OrgID,
		Name:        name,
		Description: description,
		Status:      StatusActive,
		CreatedBy:   &createdBy,
	}
	if err := s.projects.Create(ctx, p); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("create project: %w", err))
	}

	detail := &ProjectDetail{Project: *p}

	repoURL := trimmedOrNil(in.RepositoryURL)
	if repoURL != nil {
		repo, err := s.attachRepository(ctx, p.ID, *repoURL, "")
		if err != nil {
			// FR-PRJ-005: validated *before* saving. The project row is
			// already committed by this point, so a failed attach
			// compensates by removing it rather than leaving a
			// half-created project behind.
			_ = s.projects.Delete(ctx, p.ID)
			return nil, err
		}
		detail.Repository = repo
	}

	return detail, nil
}

func (s *service) List(ctx context.Context, actor domain.Actor, page Page) ([]ProjectDetail, int, error) {
	projects, total, err := s.projects.List(ctx, actor.OrgID, page)
	if err != nil {
		return nil, 0, apperrors.Internal(fmt.Errorf("list projects: %w", err))
	}

	details := make([]ProjectDetail, len(projects))
	for i, p := range projects {
		d, err := s.composeDetail(ctx, p)
		if err != nil {
			return nil, 0, err
		}
		details[i] = *d
	}
	return details, total, nil
}

func (s *service) Get(ctx context.Context, actor domain.Actor, projectID uuid.UUID) (*ProjectDetail, error) {
	p, err := s.getOwnedProject(ctx, actor, projectID)
	if err != nil {
		return nil, err
	}
	return s.composeDetail(ctx, *p)
}

func (s *service) Update(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in UpdateProjectInput) (*ProjectDetail, error) {
	p, err := s.getOwnedProject(ctx, actor, projectID)
	if err != nil {
		return nil, err
	}

	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" || len(name) > 120 {
			return nil, apperrors.Validation("project.invalid_input", "name must be between 1 and 120 characters", nil)
		}
		if existing, err := s.projects.GetByOrgAndName(ctx, actor.OrgID, name); err == nil && existing.ID != p.ID {
			return nil, apperrors.Conflict("project.name_taken", "a project with this name already exists")
		} else if err != nil && !isNotFound(err) {
			return nil, apperrors.Internal(fmt.Errorf("check existing project name: %w", err))
		}
		p.Name = name
	}
	if in.Description != nil {
		p.Description = trimmedOrNil(in.Description)
	}

	if err := s.projects.Update(ctx, p); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("update project: %w", err))
	}
	return s.composeDetail(ctx, *p)
}

func (s *service) Archive(ctx context.Context, actor domain.Actor, projectID uuid.UUID) error {
	p, err := s.getOwnedProject(ctx, actor, projectID)
	if err != nil {
		return err
	}
	p.Status = StatusArchived
	if err := s.projects.Update(ctx, p); err != nil {
		return apperrors.Internal(fmt.Errorf("archive project: %w", err))
	}
	return nil
}

func (s *service) AttachRepository(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in RepositoryInput) (*Repository, error) {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return nil, err
	}

	token, err := s.credentials.GetPlaintext(ctx, projectID, CredentialKindGitHubPAT, s.encryptionKey)
	if err != nil && !isNotFound(err) {
		return nil, apperrors.Internal(fmt.Errorf("read existing credential: %w", err))
	}

	return s.attachRepository(ctx, projectID, in.URL, token)
}

// attachRepository is the shared implementation behind Create's optional
// repository_url and the standalone AttachRepository endpoint.
func (s *service) attachRepository(ctx context.Context, projectID uuid.UUID, rawURL, token string) (*Repository, error) {
	info, err := s.vcs.ValidateRepository(ctx, rawURL, token)
	if err != nil {
		return nil, translateRepoValidationError(err, token != "")
	}

	repo := &Repository{
		ID:            id.New(),
		ProjectID:     projectID,
		Provider:      "github",
		URL:           info.NormalizedURL,
		Owner:         info.Owner,
		Name:          info.Name,
		DefaultBranch: info.DefaultBranch,
		IsPrivate:     info.IsPrivate,
		SizeKB:        &info.SizeKB,
	}
	if err := s.repositories.Upsert(ctx, repo); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("attach repository: %w", err))
	}
	return repo, nil
}

// translateRepoValidationError implements FR-PRJ-005 alongside the
// documented `project.credential_required` code
// (documentation/07-api-specification.md §1.3's error catalogue): GitHub
// answers a missing private-repo credential with the same 404 it uses for
// "doesn't exist at all" (documentation/19-client-journey-and-pricing-model.md
// §6 and adapters/github's own GetRepository doc comment) — so a 404 with
// no credential attached is reported as "attach a credential," which is the
// actionable read for a caller in that position, rather than a bare 404.
func translateRepoValidationError(err error, hadToken bool) error {
	if errors.Is(err, github.ErrInvalidRepositoryURL) {
		return apperrors.Validation("project.invalid_repository_url", "must be a valid HTTPS GitHub URL", nil)
	}
	var appErr *apperrors.Error
	if errors.As(err, &appErr) && appErr.Kind == apperrors.KindNotFound && !hadToken {
		return apperrors.Unprocessable("project.credential_required", "repository not found, or private with no credential attached — attach a GitHub personal access token and try again")
	}
	return err
}

func (s *service) SetCredential(ctx context.Context, actor domain.Actor, projectID uuid.UUID, kind, token string) (*CredentialInfo, error) {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return nil, err
	}
	if kind != CredentialKindGitHubPAT {
		return nil, apperrors.Validation("project.invalid_input", fmt.Sprintf("kind must be %q", CredentialKindGitHubPAT), nil)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, apperrors.Validation("project.invalid_input", "token is required", nil)
	}

	ciphertext, nonce, err := crypto.Encrypt(s.encryptionKey, []byte(token))
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("encrypt credential: %w", err))
	}

	updatedAt, err := s.credentials.Upsert(ctx, CredentialRow{
		ProjectID:  projectID,
		Kind:       kind,
		Ciphertext: ciphertext,
		Nonce:      nonce,
		Hint:       maskToken(token),
		CreatedBy:  actor.UserID,
	})
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("store credential: %w", err))
	}

	return &CredentialInfo{HasCredential: true, Hint: maskToken(token), UpdatedAt: &updatedAt}, nil
}

func (s *service) RemoveCredential(ctx context.Context, actor domain.Actor, projectID uuid.UUID) error {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return err
	}
	if err := s.credentials.Delete(ctx, projectID, CredentialKindGitHubPAT); err != nil && !isNotFound(err) {
		return apperrors.Internal(fmt.Errorf("remove credential: %w", err))
	}
	return nil
}

func (s *service) ListTargets(ctx context.Context, actor domain.Actor, projectID uuid.UUID) ([]Target, error) {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return nil, err
	}
	targets, err := s.targets.ListByProject(ctx, projectID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list targets: %w", err))
	}
	return targets, nil
}

func (s *service) RegisterTarget(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in TargetInput) (*Target, error) {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return nil, err
	}

	resolution, err := validate.ResolveTarget(ctx, s.resolver, in.Target, s.allowPrivateTargets, s.pentestAllowlist)
	if err != nil {
		return nil, translateTargetError(err)
	}

	pinnedIPs := make([]netip.Addr, 0, len(resolution.PinnedIPs))
	for _, ip := range resolution.PinnedIPs {
		if addr, ok := netip.AddrFromSlice(ip); ok {
			pinnedIPs = append(pinnedIPs, addr.Unmap())
		}
	}

	t := &Target{
		ID:             id.New(),
		ProjectID:      projectID,
		TargetInput:    strings.TrimSpace(in.Target),
		NormalizedHost: resolution.NormalizedHost,
		PinnedIPs:      pinnedIPs,
		Status:         TargetAwaitingAttestation,
		LastResolvedAt: time.Now().UTC(),
	}
	if err := s.targets.Create(ctx, t); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("register target: %w", err))
	}
	return t, nil
}

func translateTargetError(err error) error {
	switch {
	case errors.Is(err, validate.ErrTargetMalformed):
		return apperrors.Validation("target.invalid_input", err.Error(), nil)
	case errors.Is(err, validate.ErrTargetUnresolvable), errors.Is(err, validate.ErrTargetBlocked):
		return apperrors.Unprocessable("target.blocked_address", err.Error())
	default:
		return apperrors.Internal(err)
	}
}

func (s *service) AttestTarget(ctx context.Context, actor domain.Actor, targetID uuid.UUID, in AttestationInput) (*Target, *TargetAttestation, error) {
	t, err := s.getOwnedTarget(ctx, actor, targetID)
	if err != nil {
		return nil, nil, err
	}
	if !in.Accepted {
		return nil, nil, apperrors.Validation("target.attestation_required", "the attestation statement must be accepted", nil)
	}
	if strings.TrimSpace(in.AttestationTextVersion) == "" {
		return nil, nil, apperrors.Validation("target.attestation_required", "attestation_text_version is required", nil)
	}

	now := time.Now().UTC()
	a := &Attestation{
		ID:                     id.New(),
		TargetID:               targetID,
		UserID:                 actor.UserID,
		AttestationTextVersion: in.AttestationTextVersion,
		AcceptedAt:             now,
		SourceIP:               in.SourceIP,
	}
	if err := s.attestations.Create(ctx, a); err != nil {
		return nil, nil, apperrors.Internal(fmt.Errorf("record attestation: %w", err))
	}
	if err := s.targets.UpdateStatus(ctx, targetID, TargetAttested); err != nil {
		return nil, nil, apperrors.Internal(fmt.Errorf("update target status: %w", err))
	}
	t.Status = TargetAttested

	name, err := s.users.GetDisplayName(ctx, actor.UserID)
	if err != nil {
		return nil, nil, apperrors.Internal(fmt.Errorf("resolve attester display name: %w", err))
	}

	return t, &TargetAttestation{AttestedAt: now, AttestedByID: actor.UserID, AttestedByName: name}, nil
}

func (s *service) RevokeTarget(ctx context.Context, actor domain.Actor, targetID uuid.UUID) error {
	if _, err := s.getOwnedTarget(ctx, actor, targetID); err != nil {
		return err
	}
	if err := s.targets.UpdateStatus(ctx, targetID, TargetRevoked); err != nil {
		return apperrors.Internal(fmt.Errorf("revoke target: %w", err))
	}
	return nil
}

func (s *service) getOwnedProject(ctx context.Context, actor domain.Actor, projectID uuid.UUID) (*Project, error) {
	p, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("project.not_found", "project not found")
		}
		return nil, apperrors.Internal(fmt.Errorf("get project: %w", err))
	}
	// A project that exists but belongs to another organisation must read
	// as 404, not 403 — confirming existence is itself a leak
	// (documentation/07-api-specification.md §1.4).
	if p.OrgID != actor.OrgID {
		return nil, apperrors.NotFound("project.not_found", "project not found")
	}
	return p, nil
}

func (s *service) getOwnedTarget(ctx context.Context, actor domain.Actor, targetID uuid.UUID) (*Target, error) {
	t, err := s.targets.GetByID(ctx, targetID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("target.not_found", "target not found")
		}
		return nil, apperrors.Internal(fmt.Errorf("get target: %w", err))
	}
	if _, err := s.getOwnedProject(ctx, actor, t.ProjectID); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *service) composeDetail(ctx context.Context, p Project) (*ProjectDetail, error) {
	detail := &ProjectDetail{Project: p}

	repo, err := s.repositories.GetByProjectID(ctx, p.ID)
	switch {
	case err == nil:
		detail.Repository = repo
	case isNotFound(err):
		// no repository attached yet — not an error
	default:
		return nil, apperrors.Internal(fmt.Errorf("get repository: %w", err))
	}

	cred, err := s.credentials.GetInfo(ctx, p.ID, CredentialKindGitHubPAT)
	switch {
	case err == nil:
		detail.HasCredential = true
		detail.CredentialHint = cred.Hint
	case isNotFound(err):
		// no credential stored — not an error
	default:
		return nil, apperrors.Internal(fmt.Errorf("get credential info: %w", err))
	}

	return detail, nil
}

// maskToken produces a display hint like "ghp_••••3f9a"
// (documentation/07-api-specification.md §3) — the real token is never
// stored anywhere but the encrypted column, and never returned again.
func maskToken(token string) string {
	prefix := ""
	if idx := strings.IndexByte(token, '_'); idx > 0 && idx <= 4 {
		prefix = token[:idx+1]
		token = token[idx+1:]
	}
	if len(token) <= 4 {
		return prefix + "••••" + token
	}
	return prefix + "••••" + token[len(token)-4:]
}

func trimmedOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

func isNotFound(err error) bool {
	var appErr *apperrors.Error
	return errors.As(err, &appErr) && appErr.Kind == apperrors.KindNotFound
}

// staticResolver adapts net.DefaultResolver to validate.Resolver — kept
// here so callers wiring this module in production don't need to know
// validate's interface shape.
var _ validate.Resolver = (*net.Resolver)(nil)
