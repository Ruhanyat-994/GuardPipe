package project

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/adapters/github"
	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/audit"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/identity"
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

	// AdminRevokeTarget is modules/admin's counterpart to RevokeTarget —
	// same no-actor reasoning as GetCloneInfo below (BUILD_GUIDE.md
	// Phase 14): the caller is a platform operator acting on a confirmed
	// pentest_target_flags misuse report, not a per-request actor who owns
	// this target's project, so the usual org-ownership check doesn't
	// apply here — RequirePlatformOperator (transport/http/middleware) is
	// the authorization boundary for this one.
	AdminRevokeTarget(ctx context.Context, targetID uuid.UUID) error

	// GetAttestedTarget is modules/orchestrator's background-worker read for
	// the pentest engine — same no-actor shape as GetCloneInfo below (the
	// worker isn't handling a per-request authorization check, it's
	// processing a scan job whose creation was already authorized). A
	// project can technically hold more than one target
	// (documentation/06-database-design.md §4.7's constraint is
	// UNIQUE(project_id, normalized_host), not UNIQUE(project_id)), but
	// domain.ScanInput.Target is singular — this returns the first
	// TargetAttested-status row found, a stated simplification rather than
	// a silently arbitrary choice. Multi-target-per-scan is unsupported
	// today.
	GetAttestedTarget(ctx context.Context, projectID uuid.UUID) (*Target, error)

	ListDocuments(ctx context.Context, actor domain.Actor, projectID uuid.UUID) ([]Document, error)
	UploadDocument(ctx context.Context, actor domain.Actor, projectID uuid.UUID, filename, mimeType string, content []byte) (*Document, error)
	// ImportDocumentFromURL fetches a Google Docs/Drive share link
	// (ParseGoogleDocLink) and stores it through the same validation and
	// per-project limits as UploadDocument — the "paste a link" half of
	// docreview's document intake, alongside the "pick a file" half.
	ImportDocumentFromURL(ctx context.Context, actor domain.Actor, projectID uuid.UUID, rawURL string) (*Document, error)
	DeleteDocument(ctx context.Context, actor domain.Actor, documentID uuid.UUID) error

	// GetDocuments is docreview's (Phase 11) worker-side read — same no-actor
	// reasoning as GetCloneInfo below: the caller is a background job whose
	// creation was already authorized, not a per-request check.
	GetDocuments(ctx context.Context, projectID uuid.UUID) ([]Document, error)

	// GetCloneInfo returns what modules/orchestrator's background worker
	// needs to check out a project's repository — no actor parameter,
	// deliberately: the worker isn't handling a per-request authorization
	// check, it's processing a scan job whose creation was already
	// authorized (CreateScan verified the actor owned the project). Token
	// is "" when no credential is attached (a public repository).
	GetCloneInfo(ctx context.Context, projectID uuid.UUID) (repoURL, branch, token string, err error)

	// MarkCredentialInvalid is the worker-side counterpart to
	// GetCloneInfo — same no-actor reasoning, called from the same
	// background job-processing path when a clone comes back 401/403.
	// Never deletes or archives anything; it only flags the existing
	// repository row so the project (and its full scan history) keeps
	// showing up exactly as before, with a clear "reattach your GitHub
	// token" signal instead of every future scan silently failing with no
	// visible cause. A project with no repository attached at all is not an
	// error here — nothing to flag.
	MarkCredentialInvalid(ctx context.Context, projectID uuid.UUID, reason string) error

	// GetOrgID is modules/orchestrator's scheduler-ticker read (BUILD_GUIDE.md
	// Phase 15) — same no-actor shape as GetCloneInfo below: a scheduled
	// scan's trigger isn't a per-request authorization check, it resolves
	// which org a schedule's own project belongs to so it can build the
	// actor CreateScan needs.
	GetOrgID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error)

	// AssignProject/UnassignProject/ListAssignments are BUILD_GUIDE.md
	// Phase 15's Team Dashboard primitive — see ProjectAssignment's own doc
	// comment. AssignProject requires userID to already be a member of
	// actor's org (home or via organization_memberships) — no assigning
	// someone who hasn't accepted an invite yet.
	AssignProject(ctx context.Context, actor domain.Actor, projectID, userID uuid.UUID) (*ProjectAssignment, error)
	UnassignProject(ctx context.Context, actor domain.Actor, projectID, userID uuid.UUID) error
	ListAssignments(ctx context.Context, actor domain.Actor, projectID uuid.UUID) ([]ProjectAssignment, error)
	// ListAssignmentsForOrg is the Team Dashboard's own read — every
	// assignment across every one of actor's org's projects in one call,
	// the same "no per-project ownership check needed, the query is already
	// scoped to actor.OrgID" shape orchestrator.ListOrgScans already
	// establishes.
	ListAssignmentsForOrg(ctx context.Context, actor domain.Actor) ([]ProjectAssignment, error)

	// --- project collaborators (project-collaborators follow-up) — see
	// ProjectInvite/ProjectCollaborator's own doc comments in types.go.
	// InviteCollaborator/ListCollaboratorInvites/RevokeCollaboratorInvite/
	// ListCollaborators/RemoveCollaborator are all gated through
	// getOwnedProject like everything else on this interface — including,
	// once actor.ProjectID is set, restricted to that one project only.
	InviteCollaborator(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in InviteCollaboratorInput) (*CreatedProjectInvite, error)
	ListCollaboratorInvites(ctx context.Context, actor domain.Actor, projectID uuid.UUID) ([]ProjectInvite, error)
	RevokeCollaboratorInvite(ctx context.Context, actor domain.Actor, projectID, inviteID uuid.UUID) error
	ListCollaborators(ctx context.Context, actor domain.Actor, projectID uuid.UUID) ([]ProjectCollaborator, error)
	RemoveCollaborator(ctx context.Context, actor domain.Actor, projectID, userID uuid.UUID) error

	// ListMyProjectInvites is the live notification feed's own read —
	// mirrors organization.Service.ListMyInvites exactly, just against
	// project_invites instead of organization_invites.
	ListMyProjectInvites(ctx context.Context, actor domain.Actor) ([]PendingProjectInvite, error)
	AcceptCollaboratorInvite(ctx context.Context, actor domain.Actor, identifier string) (*ProjectCollaborator, error)
	DeclineCollaboratorInvite(ctx context.Context, actor domain.Actor, inviteID uuid.UUID) error

	// ListMyCollaborations backs the "shared projects" switcher entries —
	// every project (any org) the caller holds an accepted collaborator
	// grant on.
	ListMyCollaborations(ctx context.Context, actor domain.Actor) ([]ProjectCollaboratorSummary, error)
	// SwitchProject is `POST /auth/switch-project/{id}` — see
	// identity.Service.IssueTokenPairForProject's own doc comment for why
	// this always mints a brand-new, independent token pair. Not gated
	// through getOwnedProject (the caller, by definition, isn't in this
	// project's org) — GetByProjectAndUser against project_collaborators is
	// itself the authorization check.
	SwitchProject(ctx context.Context, actor domain.Actor, projectID uuid.UUID) (*SwitchProjectResult, error)
}

// ProjectDetail is a Project plus the pieces the API returns alongside it
// (documentation/07-api-specification.md §3's `POST /projects` example
// response) — no repository attached is a nil Repository, not an error.
type ProjectDetail struct {
	Project
	Repository     *Repository
	HasCredential  bool
	CredentialHint string
	// HasAttestedTarget lets a caller (orchestrator.resolveEngines, the
	// frontend's engine picker) know whether pentest can run here without a
	// second GetAttestedTarget/GET .../targets round trip — cheap to compute
	// alongside Repository/HasCredential in composeDetail, so it's included
	// unconditionally rather than behind a separate lookup.
	HasAttestedTarget bool
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
	// §4.5). Also clears CredentialInvalidAt/Reason back to nil — an attach
	// or replace is exactly the "user just fixed it" signal.
	Upsert(ctx context.Context, r *Repository) error
	GetByProjectID(ctx context.Context, projectID uuid.UUID) (*Repository, error)
	// MarkCredentialInvalid flags the project's repository row — called by
	// the orchestrator worker (via Service.MarkCredentialInvalid) the moment
	// a scan's clone is rejected with 401/403, so the state survives between
	// scans instead of only ever showing up as one scan's failure reason.
	MarkCredentialInvalid(ctx context.Context, projectID uuid.UUID, reason string, at time.Time) error
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

// DocumentRepository is defined by this package.
type DocumentRepository interface {
	Create(ctx context.Context, d *Document) error
	GetByID(ctx context.Context, id uuid.UUID) (*Document, error)
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]Document, error)
	CountByProject(ctx context.Context, projectID uuid.UUID) (int, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// Document upload limits (documentation/05-module-specifications.md §11) —
// per project, not per scan: a document is uploaded once and reused by
// every scan against that project, not re-uploaded each time.
const (
	maxDocumentSizeBytes   = 100 * 1024
	maxDocumentsPerProject = 20
)

// allowedDocumentExtensions extends §11's original Core allowlist with
// .pdf and .csv. PDF is the one type that isn't already plain text — it's
// routed through pdfExtractor in UploadDocument before ever reaching
// createDocument's size/count checks, one new dependency
// (github.com/ledongthuc/pdf, pure Go, MIT) justified specifically for that.
var allowedDocumentExtensions = map[string]bool{
	".md": true, ".txt": true, ".adoc": true, ".rst": true, ".pdf": true, ".csv": true,
}

// ProjectAssignmentRepository is defined by this package; implementation
// lives in internal/store/repo.
type ProjectAssignmentRepository interface {
	Create(ctx context.Context, a *ProjectAssignment) error
	Delete(ctx context.Context, projectID, userID uuid.UUID) error
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]ProjectAssignment, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]ProjectAssignment, error)
}

// MembershipChecker is the narrow slice of modules/organization this
// package needs — AssignProject's "assignee must already be an org member"
// rule (BUILD_GUIDE.md Phase 15). Implemented in internal/store/repo against
// organization_memberships/users, the tables modules/organization and
// modules/identity respectively own — this package only ever reads through
// this one method, the same "define the interface you need, implemented by
// store/repo" pattern UserDisplayNameLookup already establishes.
type MembershipChecker interface {
	IsOrgMember(ctx context.Context, orgID, userID uuid.UUID) (bool, error)
}

// UserDisplayNameLookup is what this module needs from `identity` — the
// attesting user's display name for the attestation response
// (documentation/07-api-specification.md §4's "attested_by"), plus (project-
// collaborators follow-up) the caller's own email, to match an invite's
// Email against, the same purpose organization.Service's own UserReader
// interface serves for org invites. The module dependency table
// (documentation/05-module-specifications.md §1) lists
// `project → identity, vcs`, so this dependency is intentional, not a
// layering violation.
type UserDisplayNameLookup interface {
	GetDisplayName(ctx context.Context, userID uuid.UUID) (string, error)
	GetEmail(ctx context.Context, userID uuid.UUID) (string, error)
}

// OrganizationNameLookup is the one thing this module needs from
// modules/organization (project-collaborators follow-up) — an owning org's
// display name, purely for PendingProjectInvite/ProjectCollaboratorSummary
// rendering. Implemented in internal/store/repo against `organizations`, the
// same GetName shape organization.OrganizationReader already establishes.
type OrganizationNameLookup interface {
	GetName(ctx context.Context, orgID uuid.UUID) (string, error)
}

type service struct {
	projects      ProjectRepository
	repositories  RepositoryRepository
	credentials   CredentialRepository
	targets       TargetRepository
	attestations  AttestationRepository
	documents     DocumentRepository
	assignments   ProjectAssignmentRepository
	invites       ProjectInviteRepository
	collaborators ProjectCollaboratorRepository
	membership    MembershipChecker
	users         UserDisplayNameLookup
	orgs          OrganizationNameLookup
	identitySvc   identity.Service
	vcs           vcs.Service
	resolver      validate.Resolver
	audit         audit.Service
	urlFetcher    URLFetcher
	pdfExtractor  PDFTextExtractor
	encryptionKey []byte

	allowPrivateTargets bool
	pentestDenylist     []string
	attestationVersion  string
}

// NewService wires the project module. auditSvc is BUILD_GUIDE.md Phase 6's
// retroactive instrumentation — project created/archived, repository
// attached, target attested are the four events named there.
func NewService(
	projects ProjectRepository,
	repositories RepositoryRepository,
	credentials CredentialRepository,
	targets TargetRepository,
	attestations AttestationRepository,
	documents DocumentRepository,
	assignments ProjectAssignmentRepository,
	invites ProjectInviteRepository,
	collaborators ProjectCollaboratorRepository,
	membership MembershipChecker,
	users UserDisplayNameLookup,
	orgs OrganizationNameLookup,
	identitySvc identity.Service,
	vcsSvc vcs.Service,
	resolver validate.Resolver,
	auditSvc audit.Service,
	urlFetcher URLFetcher,
	pdfExtractor PDFTextExtractor,
	encryptionKey []byte,
	allowPrivateTargets bool,
	pentestDenylist []string,
) Service {
	return &service{
		projects:            projects,
		repositories:        repositories,
		credentials:         credentials,
		targets:             targets,
		attestations:        attestations,
		documents:           documents,
		assignments:         assignments,
		invites:             invites,
		collaborators:       collaborators,
		membership:          membership,
		users:               users,
		orgs:                orgs,
		identitySvc:         identitySvc,
		vcs:                 vcsSvc,
		resolver:            resolver,
		audit:               auditSvc,
		urlFetcher:          urlFetcher,
		pdfExtractor:        pdfExtractor,
		encryptionKey:       encryptionKey,
		allowPrivateTargets: allowPrivateTargets,
		pentestDenylist:     pentestDenylist,
		attestationVersion:  "v1",
	}
}

func (s *service) Create(ctx context.Context, actor domain.Actor, in CreateProjectInput) (*ProjectDetail, error) {
	if err := requireUnscoped(actor); err != nil {
		return nil, err
	}
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
	s.audit.Log(ctx, audit.Entry{
		OrgID: &actor.OrgID, ActorID: &actor.UserID, Action: "project.created",
		ResourceType: strPtr("project"), ResourceID: &p.ID,
	})

	detail := &ProjectDetail{Project: *p}

	repoURL := trimmedOrNil(in.RepositoryURL)
	if repoURL != nil {
		repo, err := s.attachRepository(ctx, actor, p.ID, *repoURL, "")
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
	// A project-scoped collaborator session's "my projects" list is just
	// the one project it was switched into — never the rest of that
	// project's org — so it needs no frontend-visible branching: the same
	// GET /projects call a home session makes just returns a single-item
	// result.
	if actor.ProjectID != nil {
		d, err := s.Get(ctx, actor, *actor.ProjectID)
		if err != nil {
			return nil, 0, err
		}
		return []ProjectDetail{*d}, 1, nil
	}

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
	s.audit.Log(ctx, audit.Entry{
		OrgID: &actor.OrgID, ActorID: &actor.UserID, Action: "project.archived",
		ResourceType: strPtr("project"), ResourceID: &projectID,
	})
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

	return s.attachRepository(ctx, actor, projectID, in.URL, token)
}

// attachRepository is the shared implementation behind Create's optional
// repository_url and the standalone AttachRepository endpoint — actor is
// threaded through just for the audit entry, both call sites already have
// one in scope.
func (s *service) attachRepository(ctx context.Context, actor domain.Actor, projectID uuid.UUID, rawURL, token string) (*Repository, error) {
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
	s.audit.Log(ctx, audit.Entry{
		OrgID: &actor.OrgID, ActorID: &actor.UserID, Action: "repository.attached",
		ResourceType: strPtr("project"), ResourceID: &projectID,
		Detail: map[string]any{"repository_url": repo.URL},
	})
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

// GetAttestedTarget is the worker-side counterpart to ListTargets — see the
// Service interface's own doc comment for the no-actor reasoning and the
// stated multi-target simplification.
func (s *service) GetAttestedTarget(ctx context.Context, projectID uuid.UUID) (*Target, error) {
	targets, err := s.targets.ListByProject(ctx, projectID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list targets: %w", err))
	}
	for _, t := range targets {
		if t.Status == TargetAttested {
			target := t
			return &target, nil
		}
	}
	return nil, apperrors.NotFound("project.pentest_target_not_found", "no attested pentest target attached to this project")
}

func (s *service) RegisterTarget(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in TargetInput) (*Target, error) {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return nil, err
	}

	resolution, err := validate.ResolveTarget(ctx, s.resolver, in.Target, s.allowPrivateTargets, s.pentestDenylist)
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

	var ipAddr *netip.Addr
	if parsed, err := netip.ParseAddr(in.SourceIP); err == nil {
		ipAddr = &parsed
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &actor.OrgID, ActorID: &actor.UserID, Action: "target.attested",
		ResourceType: strPtr("pentest_target"), ResourceID: &targetID, IP: ipAddr,
	})

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

func (s *service) AdminRevokeTarget(ctx context.Context, targetID uuid.UUID) error {
	if err := s.targets.UpdateStatus(ctx, targetID, TargetRevoked); err != nil {
		return apperrors.Internal(fmt.Errorf("admin revoke target: %w", err))
	}
	return nil
}

func (s *service) ListDocuments(ctx context.Context, actor domain.Actor, projectID uuid.UUID) ([]Document, error) {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return nil, err
	}
	docs, err := s.documents.ListByProject(ctx, projectID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list documents: %w", err))
	}
	return docs, nil
}

func (s *service) UploadDocument(ctx context.Context, actor domain.Actor, projectID uuid.UUID, filename, mimeType string, content []byte) (*Document, error) {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return nil, err
	}

	filename = strings.TrimSpace(filename)
	if filename == "" {
		return nil, apperrors.Validation("document.invalid_input", "filename is required", nil)
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if !allowedDocumentExtensions[ext] {
		return nil, apperrors.Validation("document.unsupported_type", "filename must end in .md, .txt, .adoc, .rst, .pdf, or .csv", nil)
	}
	if len(content) == 0 {
		return nil, apperrors.Validation("document.invalid_input", "file is empty", nil)
	}

	// Every other allowed extension is already plain text; a PDF is the one
	// binary format in the allowlist, so its bytes are swapped out for the
	// extracted text here — createDocument's size/count checks then apply
	// to that extracted text, not the (usually much larger) raw PDF.
	if ext == ".pdf" {
		text, err := s.pdfExtractor.ExtractText(content)
		if err != nil {
			return nil, apperrors.Validation("document.invalid_pdf", "could not read this PDF — it may be corrupted or password-protected", nil)
		}
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, apperrors.Validation("document.invalid_pdf", "no extractable text found in this PDF — scanned/image-only PDFs aren't supported", nil)
		}
		content = []byte(text)
		mimeType = "text/plain"
	}

	return s.createDocument(ctx, actor, projectID, filename, mimeType, content, "upload")
}

// ImportDocumentFromURL is the "paste a link" counterpart to UploadDocument
// — it fetches a Google Docs/Drive share link server-side rather than
// receiving bytes over the request, then runs through the exact same size
// and per-project limit checks via createDocument. No new Google Cloud
// credentials are needed: the link must already be shared "anyone with the
// link can view," and the fetch is a plain HTTPS GET against Google's public
// export endpoint — no OAuth, no API key.
func (s *service) ImportDocumentFromURL(ctx context.Context, actor domain.Actor, projectID uuid.UUID, rawURL string) (*Document, error) {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return nil, err
	}

	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, apperrors.Validation("document.invalid_input", "url is required", nil)
	}
	exportURL, filename, err := ParseGoogleDocLink(rawURL)
	if err != nil {
		return nil, apperrors.Validation("document.invalid_google_url", err.Error(), nil)
	}

	contentType, body, err := s.urlFetcher.Fetch(ctx, exportURL)
	if err != nil {
		return nil, apperrors.Unprocessable("document.import_failed", "could not fetch the document — check the link and try again")
	}
	// A link that isn't shared "anyone with the link can view" resolves to
	// Google's HTML sign-in/permission page instead of the plain-text
	// export — that's the one signal available without OAuth to tell "not
	// public" apart from "public but empty."
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return nil, apperrors.Unprocessable("document.not_publicly_accessible", `this document isn't shared publicly — set sharing to "anyone with the link can view" and try again`)
	}
	if len(body) == 0 {
		return nil, apperrors.Validation("document.invalid_input", "the imported document is empty", nil)
	}

	return s.createDocument(ctx, actor, projectID, filename, "text/plain", body, "google_drive")
}

// createDocument is UploadDocument and ImportDocumentFromURL's shared tail:
// enforce the size and per-project count limits, persist, and audit-log.
// Callers are responsible for filename/extension validation and for making
// sure content is non-empty before calling this.
func (s *service) createDocument(ctx context.Context, actor domain.Actor, projectID uuid.UUID, filename, mimeType string, content []byte, source string) (*Document, error) {
	if len(content) > maxDocumentSizeBytes {
		return nil, apperrors.Validation("document.too_large", fmt.Sprintf("file is larger than the %d KB limit", maxDocumentSizeBytes/1024), nil)
	}

	count, err := s.documents.CountByProject(ctx, projectID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("count documents: %w", err))
	}
	if count >= maxDocumentsPerProject {
		return nil, apperrors.Unprocessable("document.limit_reached", fmt.Sprintf("this project already has the maximum of %d documents — delete one before uploading another", maxDocumentsPerProject))
	}

	uploadedBy := actor.UserID
	d := &Document{
		ID: id.New(), ProjectID: projectID, UploadedBy: &uploadedBy,
		Filename: filename, MIMEType: mimeType, SizeBytes: len(content), Content: content,
	}
	if err := s.documents.Create(ctx, d); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("upload document: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &actor.OrgID, ActorID: &actor.UserID, Action: "document.uploaded",
		ResourceType: strPtr("project"), ResourceID: &projectID,
		Detail: map[string]any{"filename": d.Filename, "source": source},
	})
	return d, nil
}

func (s *service) DeleteDocument(ctx context.Context, actor domain.Actor, documentID uuid.UUID) error {
	if _, err := s.getOwnedDocument(ctx, actor, documentID); err != nil {
		return err
	}
	if err := s.documents.Delete(ctx, documentID); err != nil {
		return apperrors.Internal(fmt.Errorf("delete document: %w", err))
	}
	return nil
}

// GetDocuments is docreview's worker-side read — no ownership check,
// deliberately (see the Service interface's own doc comment on this
// method): the caller is background job processing, not a per-request
// check.
func (s *service) GetDocuments(ctx context.Context, projectID uuid.UUID) ([]Document, error) {
	docs, err := s.documents.ListByProject(ctx, projectID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("get documents: %w", err))
	}
	return docs, nil
}

func (s *service) GetCloneInfo(ctx context.Context, projectID uuid.UUID) (repoURL, branch, token string, err error) {
	repo, err := s.repositories.GetByProjectID(ctx, projectID)
	if err != nil {
		if isNotFound(err) {
			return "", "", "", apperrors.NotFound("project.repository_not_found", "no repository attached to this project")
		}
		return "", "", "", apperrors.Internal(fmt.Errorf("get repository: %w", err))
	}

	plaintext, err := s.credentials.GetPlaintext(ctx, projectID, CredentialKindGitHubPAT, s.encryptionKey)
	if err != nil && !isNotFound(err) {
		return "", "", "", apperrors.Internal(fmt.Errorf("read credential: %w", err))
	}

	return repo.URL, repo.DefaultBranch, plaintext, nil
}

func (s *service) MarkCredentialInvalid(ctx context.Context, projectID uuid.UUID, reason string) error {
	if err := s.repositories.MarkCredentialInvalid(ctx, projectID, reason, time.Now().UTC()); err != nil {
		if isNotFound(err) {
			return nil // no repository attached — nothing to flag
		}
		return apperrors.Internal(fmt.Errorf("mark credential invalid: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		ResourceType: strPtr("project"), ResourceID: &projectID,
		Action: "repository.credential_invalidated",
		Detail: map[string]any{"reason": reason},
	})
	return nil
}

func (s *service) GetOrgID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	p, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		if isNotFound(err) {
			return uuid.Nil, apperrors.NotFound("project.not_found", "project not found")
		}
		return uuid.Nil, apperrors.Internal(fmt.Errorf("get project: %w", err))
	}
	return p.OrgID, nil
}

func (s *service) AssignProject(ctx context.Context, actor domain.Actor, projectID, userID uuid.UUID) (*ProjectAssignment, error) {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return nil, err
	}
	member, err := s.membership.IsOrgMember(ctx, actor.OrgID, userID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("check org membership: %w", err))
	}
	if !member {
		return nil, apperrors.Validation("project.not_org_member", "assignee must already be a member of your organization", nil)
	}

	assignedBy := actor.UserID
	a := &ProjectAssignment{ID: id.New(), ProjectID: projectID, UserID: userID, AssignedBy: &assignedBy}
	if err := s.assignments.Create(ctx, a); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("assign project: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &actor.OrgID, ActorID: &actor.UserID, Action: "project.assigned",
		ResourceType: strPtr("project"), ResourceID: &projectID,
		Detail: map[string]any{"user_id": userID.String()},
	})
	return a, nil
}

func (s *service) UnassignProject(ctx context.Context, actor domain.Actor, projectID, userID uuid.UUID) error {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return err
	}
	if err := s.assignments.Delete(ctx, projectID, userID); err != nil {
		return apperrors.Internal(fmt.Errorf("unassign project: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &actor.OrgID, ActorID: &actor.UserID, Action: "project.unassigned",
		ResourceType: strPtr("project"), ResourceID: &projectID,
		Detail: map[string]any{"user_id": userID.String()},
	})
	return nil
}

func (s *service) ListAssignments(ctx context.Context, actor domain.Actor, projectID uuid.UUID) ([]ProjectAssignment, error) {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return nil, err
	}
	list, err := s.assignments.ListByProject(ctx, projectID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list assignments: %w", err))
	}
	return list, nil
}

func (s *service) ListAssignmentsForOrg(ctx context.Context, actor domain.Actor) ([]ProjectAssignment, error) {
	if err := requireUnscoped(actor); err != nil {
		return nil, err
	}
	list, err := s.assignments.ListByOrg(ctx, actor.OrgID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list org assignments: %w", err))
	}
	return list, nil
}

// requireUnscoped rejects a project-collaborator session (actor.ProjectID
// set) from any operation with no single project to check it against —
// creating a brand-new project, or reading the org-wide Team Dashboard. A
// scoped session may only ever touch the one project named in its token
// (enforced in getOwnedProject below) plus whatever hangs off it; it must
// never browse or create anything else in that project's organisation.
func requireUnscoped(actor domain.Actor) error {
	if actor.ProjectID != nil {
		return apperrors.Forbidden("project.scoped_session", "this session is scoped to a single shared project")
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
	// A project-collaborator session (project-collaborators follow-up) may
	// only ever touch the one project it was switched into — this is the
	// single choke point every project/scan/finding/document/target/report
	// authorisation check in the codebase funnels through (getOwnedTarget/
	// getOwnedDocument below, orchestrator.getOwnedScan via projects.Get),
	// so this one guard is what makes the scoped session unable to reach
	// any other project in the same org, even though p.OrgID == actor.OrgID
	// already passed above.
	if actor.ProjectID != nil && *actor.ProjectID != projectID {
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

func (s *service) getOwnedDocument(ctx context.Context, actor domain.Actor, documentID uuid.UUID) (*Document, error) {
	d, err := s.documents.GetByID(ctx, documentID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("document.not_found", "document not found")
		}
		return nil, apperrors.Internal(fmt.Errorf("get document: %w", err))
	}
	if _, err := s.getOwnedProject(ctx, actor, d.ProjectID); err != nil {
		return nil, err
	}
	return d, nil
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

	if _, err := s.GetAttestedTarget(ctx, p.ID); err == nil {
		detail.HasAttestedTarget = true
	} else if !isNotFound(err) {
		return nil, apperrors.Internal(fmt.Errorf("check attested target: %w", err))
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

// projectInviteTTL mirrors organization's own inviteTTL constant (Phase 15) —
// no requirement doc specifies one, same conservative default.
const projectInviteTTL = 7 * 24 * time.Hour

func (s *service) InviteCollaborator(ctx context.Context, actor domain.Actor, projectID uuid.UUID, in InviteCollaboratorInput) (*CreatedProjectInvite, error) {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return nil, err
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" {
		return nil, apperrors.Validation("project.invalid_input", "email is required", nil)
	}
	if !in.Role.Valid() {
		return nil, apperrors.Validation("project.invalid_input", "role must be a recognised role", nil)
	}

	// Whether email already maps to an existing collaborator can't be
	// checked here — InviteCollaborator only has an email, not a resolved
	// user id (a project_collaborators row is keyed by user_id, and the
	// invitee may not even have an account yet). AcceptCollaboratorInvite
	// re-checks by resolved user id once the invite is actually accepted.
	if pending, err := s.invites.ExistsPending(ctx, projectID, email); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("check pending project invite: %w", err))
	} else if pending {
		return nil, apperrors.Conflict("project.invite_already_pending", "an invite is already pending for this email")
	}

	rawToken, tokenHash, err := newProjectInviteToken()
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("generate invite token: %w", err))
	}

	invitedBy := actor.UserID
	inv := &ProjectInvite{
		ID: id.New(), ProjectID: projectID, Email: email, Role: in.Role,
		InvitedBy: &invitedBy, TokenHash: tokenHash, Status: InvitePending,
		ExpiresAt: time.Now().UTC().Add(projectInviteTTL),
	}
	if err := s.invites.Create(ctx, inv); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("create project invite: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &actor.OrgID, ActorID: &actor.UserID, Action: "project.collaborator_invited",
		ResourceType: strPtr("project"), ResourceID: &projectID,
		Detail: map[string]any{"email": email, "role": string(in.Role)},
	})
	return &CreatedProjectInvite{ProjectInvite: *inv, RawToken: rawToken}, nil
}

func (s *service) ListCollaboratorInvites(ctx context.Context, actor domain.Actor, projectID uuid.UUID) ([]ProjectInvite, error) {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return nil, err
	}
	list, err := s.invites.ListByProject(ctx, projectID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list project invites: %w", err))
	}
	return list, nil
}

func (s *service) RevokeCollaboratorInvite(ctx context.Context, actor domain.Actor, projectID, inviteID uuid.UUID) error {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return err
	}
	inv, err := s.invites.GetByID(ctx, inviteID)
	if err != nil {
		if isNotFound(err) {
			return apperrors.NotFound("project.invite_not_found", "invite not found")
		}
		return apperrors.Internal(fmt.Errorf("get project invite: %w", err))
	}
	if inv.ProjectID != projectID {
		return apperrors.NotFound("project.invite_not_found", "invite not found")
	}
	if err := s.invites.SetStatus(ctx, inviteID, InviteRevoked); err != nil {
		return apperrors.Internal(fmt.Errorf("revoke project invite: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &actor.OrgID, ActorID: &actor.UserID, Action: "project.collaborator_invite_revoked",
		ResourceType: strPtr("project"), ResourceID: &projectID,
	})
	return nil
}

func (s *service) ListCollaborators(ctx context.Context, actor domain.Actor, projectID uuid.UUID) ([]ProjectCollaborator, error) {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return nil, err
	}
	list, err := s.collaborators.ListByProject(ctx, projectID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list project collaborators: %w", err))
	}
	return list, nil
}

func (s *service) RemoveCollaborator(ctx context.Context, actor domain.Actor, projectID, userID uuid.UUID) error {
	if _, err := s.getOwnedProject(ctx, actor, projectID); err != nil {
		return err
	}
	if err := s.collaborators.Delete(ctx, projectID, userID); err != nil {
		return apperrors.Internal(fmt.Errorf("remove project collaborator: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &actor.OrgID, ActorID: &actor.UserID, Action: "project.collaborator_removed",
		ResourceType: strPtr("project"), ResourceID: &projectID,
		Detail: map[string]any{"user_id": userID.String()},
	})
	return nil
}

func (s *service) ListMyProjectInvites(ctx context.Context, actor domain.Actor) ([]PendingProjectInvite, error) {
	email, err := s.callerEmail(ctx, actor)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("get caller: %w", err))
	}

	invites, err := s.invites.ListByEmail(ctx, email)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list project invites by email: %w", err))
	}

	now := time.Now().UTC()
	out := make([]PendingProjectInvite, 0, len(invites))
	for _, inv := range invites {
		if inv.Status != InvitePending {
			continue
		}
		if inv.ExpiresAt.Before(now) {
			_ = s.invites.SetStatus(ctx, inv.ID, InviteExpired)
			continue
		}
		p, err := s.projects.GetByID(ctx, inv.ProjectID)
		if err != nil {
			if isNotFound(err) {
				continue // the project was deleted out from under a still-pending invite
			}
			return nil, apperrors.Internal(fmt.Errorf("get invited project: %w", err))
		}
		orgName, err := s.orgName(ctx, p.OrgID)
		if err != nil {
			return nil, err
		}
		out = append(out, PendingProjectInvite{ProjectInvite: inv, ProjectName: p.Name, OrgName: orgName})
	}
	return out, nil
}

func (s *service) AcceptCollaboratorInvite(ctx context.Context, actor domain.Actor, identifier string) (*ProjectCollaborator, error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return nil, apperrors.Validation("project.invalid_input", "token is required", nil)
	}

	var inv *ProjectInvite
	var err error
	if inviteID, parseErr := uuid.Parse(identifier); parseErr == nil {
		inv, err = s.invites.GetByID(ctx, inviteID)
	} else {
		inv, err = s.invites.GetByTokenHash(ctx, hashProjectInviteToken(identifier))
	}
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("project.invite_not_found", "invite not found or already used")
		}
		return nil, apperrors.Internal(fmt.Errorf("get project invite: %w", err))
	}

	if inv.Status != InvitePending {
		return nil, apperrors.Unprocessable("project.invite_not_pending", "this invite is no longer valid")
	}
	if inv.ExpiresAt.Before(time.Now().UTC()) {
		_ = s.invites.SetStatus(ctx, inv.ID, InviteExpired)
		return nil, apperrors.Unprocessable("project.invite_expired", "this invite has expired")
	}

	callerEmail, err := s.callerEmail(ctx, actor)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("get caller: %w", err))
	}
	if !strings.EqualFold(callerEmail, inv.Email) {
		return nil, apperrors.Forbidden("project.invite_email_mismatch", "this invite was sent to a different email address — log in with that account to accept it")
	}
	if existing, err := s.collaborators.GetByProjectAndUser(ctx, inv.ProjectID, actor.UserID); err != nil && !isNotFound(err) {
		return nil, apperrors.Internal(fmt.Errorf("check existing collaborator: %w", err))
	} else if existing != nil {
		return nil, apperrors.Conflict("project.already_collaborator", "you already have access to this project")
	}

	c := &ProjectCollaborator{ID: id.New(), ProjectID: inv.ProjectID, UserID: actor.UserID, Role: inv.Role, InvitedBy: inv.InvitedBy}
	if err := s.collaborators.Create(ctx, c); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("create project collaborator: %w", err))
	}
	if err := s.invites.SetStatus(ctx, inv.ID, InviteAccepted); err != nil {
		return nil, apperrors.Internal(fmt.Errorf("mark project invite accepted: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		ActorID: &actor.UserID, Action: "project.collaborator_invite_accepted",
		ResourceType: strPtr("project"), ResourceID: &inv.ProjectID,
	})
	return c, nil
}

func (s *service) DeclineCollaboratorInvite(ctx context.Context, actor domain.Actor, inviteID uuid.UUID) error {
	inv, err := s.invites.GetByID(ctx, inviteID)
	if err != nil {
		if isNotFound(err) {
			return apperrors.NotFound("project.invite_not_found", "invite not found")
		}
		return apperrors.Internal(fmt.Errorf("get project invite: %w", err))
	}
	if inv.Status != InvitePending {
		return apperrors.Unprocessable("project.invite_not_pending", "this invite is no longer pending")
	}
	callerEmail, err := s.callerEmail(ctx, actor)
	if err != nil {
		return apperrors.Internal(fmt.Errorf("get caller: %w", err))
	}
	if !strings.EqualFold(callerEmail, inv.Email) {
		return apperrors.Forbidden("project.invite_email_mismatch", "this invite was sent to a different email address")
	}
	if err := s.invites.SetStatus(ctx, inviteID, InviteDeclined); err != nil {
		return apperrors.Internal(fmt.Errorf("decline project invite: %w", err))
	}
	s.audit.Log(ctx, audit.Entry{
		ActorID: &actor.UserID, Action: "project.collaborator_invite_declined",
		ResourceType: strPtr("project"), ResourceID: &inv.ProjectID,
	})
	return nil
}

func (s *service) ListMyCollaborations(ctx context.Context, actor domain.Actor) ([]ProjectCollaboratorSummary, error) {
	grants, err := s.collaborators.ListByUser(ctx, actor.UserID)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("list project collaborations: %w", err))
	}
	out := make([]ProjectCollaboratorSummary, 0, len(grants))
	for _, g := range grants {
		p, err := s.projects.GetByID(ctx, g.ProjectID)
		if err != nil {
			if isNotFound(err) {
				continue // the project was deleted out from under an accepted grant
			}
			return nil, apperrors.Internal(fmt.Errorf("get shared project: %w", err))
		}
		orgName, err := s.orgName(ctx, p.OrgID)
		if err != nil {
			return nil, err
		}
		out = append(out, ProjectCollaboratorSummary{
			ProjectID: p.ID, ProjectName: p.Name, OrgID: p.OrgID, OrgName: orgName, Role: g.Role,
		})
	}
	return out, nil
}

func (s *service) SwitchProject(ctx context.Context, actor domain.Actor, projectID uuid.UUID) (*SwitchProjectResult, error) {
	grant, err := s.collaborators.GetByProjectAndUser(ctx, projectID, actor.UserID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.Forbidden("project.not_a_collaborator", "you do not have access to this project")
		}
		return nil, apperrors.Internal(fmt.Errorf("get project collaborator grant: %w", err))
	}
	p, err := s.projects.GetByID(ctx, projectID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperrors.NotFound("project.not_found", "project not found")
		}
		return nil, apperrors.Internal(fmt.Errorf("get project: %w", err))
	}

	pair, err := s.identitySvc.IssueTokenPairForProject(ctx, actor.UserID, projectID, p.OrgID, grant.Role)
	if err != nil {
		return nil, err
	}
	s.audit.Log(ctx, audit.Entry{
		OrgID: &p.OrgID, ActorID: &actor.UserID, Action: "auth.project_switched",
		ResourceType: strPtr("project"), ResourceID: &projectID,
	})
	return &SwitchProjectResult{AccessToken: pair.AccessToken, RefreshToken: pair.RefreshToken, ExpiresIn: pair.ExpiresIn}, nil
}

// callerEmail resolves actor's own account email — Accept/Decline/
// ListMyProjectInvites all need it to match against a ProjectInvite's Email,
// the same purpose organization.Service's caller-email lookup serves for
// org invites.
func (s *service) callerEmail(ctx context.Context, actor domain.Actor) (string, error) {
	return s.users.GetEmail(ctx, actor.UserID)
}

// orgName resolves an owning org's display name for
// ListMyProjectInvites/ListMyCollaborations — both need it purely for
// display, the same reason organization.PendingInvite.OrgName exists.
func (s *service) orgName(ctx context.Context, orgID uuid.UUID) (string, error) {
	name, err := s.orgs.GetName(ctx, orgID)
	if err != nil {
		return "", apperrors.Internal(fmt.Errorf("get organization name: %w", err))
	}
	return name, nil
}

func newProjectInviteToken() (raw, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("generate random bytes: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashProjectInviteToken(raw), nil
}

func hashProjectInviteToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func strPtr(s string) *string {
	return &s
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
