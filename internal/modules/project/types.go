// Package project owns projects, repositories, pentest targets, and
// encrypted credentials (documentation/05-module-specifications.md §4). It
// depends on `identity` (to resolve a display name for a target
// attestation) and `vcs` (to validate/clone a repository), and nothing else
// — orchestrator and every engine depend on it, never the other way.
package project

import (
	"net/netip"
	"time"

	"github.com/google/uuid"
)

// Status mirrors the `project_status` Postgres enum
// (documentation/06-database-design.md §3).
type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

// Project mirrors the `projects` table (documentation/06-database-design.md
// §4.4).
type Project struct {
	ID          uuid.UUID
	OrgID       uuid.UUID
	Name        string
	Description *string
	Status      Status
	CreatedBy   *uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Repository mirrors the `repositories` table
// (documentation/06-database-design.md §4.5). One per project in this
// release — attaching a new one replaces it. CredentialInvalidAt is nil
// while the credential is healthy (or has never been checked); the
// orchestrator sets it the moment a scan's clone is rejected with 401/403,
// and the existing attach/replace flow (RepositoryRepository.Upsert) clears
// it back to nil the next time a credential is (re)attached — the project
// and every past scan/finding are untouched either way, nothing here ever
// deletes or archives anything.
type Repository struct {
	ID                      uuid.UUID
	ProjectID               uuid.UUID
	Provider                string
	URL                     string
	Owner                   string
	Name                    string
	DefaultBranch           string
	IsPrivate               bool
	SizeKB                  *int64
	LastValidatedAt         *time.Time
	CredentialInvalidAt     *time.Time
	CredentialInvalidReason *string
}

// CredentialKindGitHubPAT is the only credential kind this release supports
// (documentation/06-database-design.md §4.6's CHECK constraint).
const CredentialKindGitHubPAT = "github_pat"

// CredentialInfo is everything about a stored credential that's safe to
// return over the API — never the plaintext token, never the ciphertext
// (FR-PRJ-004).
type CredentialInfo struct {
	HasCredential bool
	Hint          string
	UpdatedAt     *time.Time
}

// TargetStatus mirrors the `target_status` Postgres enum
// (documentation/06-database-design.md §3).
type TargetStatus string

const (
	TargetAwaitingAttestation TargetStatus = "awaiting_attestation"
	TargetAttested            TargetStatus = "attested"
	TargetBlocked             TargetStatus = "blocked"
	TargetRevoked             TargetStatus = "revoked"
)

// Target mirrors the `pentest_targets` table
// (documentation/06-database-design.md §4.7).
type Target struct {
	ID             uuid.UUID
	ProjectID      uuid.UUID
	TargetInput    string
	NormalizedHost string
	// PinnedIPs is net/netip rather than net.IP because pgx/v5 maps
	// Postgres `inet`/`inet[]` to netip.Addr, not net.IP
	// (documentation/06-database-design.md §4.7's `pinned_ips INET[]`).
	PinnedIPs      []netip.Addr
	Status         TargetStatus
	LastResolvedAt time.Time
}

// Attestation mirrors the `target_attestations` table
// (documentation/06-database-design.md §4.8) — append-only, never updated
// or deleted.
type Attestation struct {
	ID                     uuid.UUID
	TargetID               uuid.UUID
	UserID                 uuid.UUID
	AttestationTextVersion string
	AcceptedAt             time.Time
	SourceIP               string
}

// TargetAttestation is the read-side view AttestTarget returns: the
// attestation plus the attesting user's display name, which lives in
// `identity`'s tables, not this module's (documentation/07-api-specification.md
// §4's "attested_by": {"id", "display_name"}).
type TargetAttestation struct {
	AttestedAt     time.Time
	AttestedByID   uuid.UUID
	AttestedByName string
}

// Document mirrors the `documents` table (Phase 11, docreview) — a design/
// requirements document uploaded directly to a project, reviewed alongside
// anything docreview discovers inside the repository checkout itself.
// Content is the raw file bytes for every text-based extension (.md/.txt/
// .adoc/.rst/.csv); for a .pdf upload it's the extracted plain text
// instead — the original PDF binary is never stored (service.go's
// UploadDocument runs it through PDFTextExtractor before this is written).
// Never returned over the API, only read by the orchestrator worker at scan
// time.
type Document struct {
	ID         uuid.UUID
	ProjectID  uuid.UUID
	UploadedBy *uuid.UUID
	Filename   string
	MIMEType   string
	SizeBytes  int
	Content    []byte
	CreatedAt  time.Time
}

// Page is a 1-based page request (documentation/07-api-specification.md
// §1.5).
type Page struct {
	Page     int
	PageSize int
}

// ProjectAssignment mirrors one row of `project_assignments` (migration
// 00021, BUILD_GUIDE.md Phase 15) — the Team Dashboard's "which developer is
// on which project" join. Assigning a project to a teammate doesn't grant
// them any extra access beyond their existing org role (RBAC is still the
// org-membership role, viewer/member/admin) — it's a workload/visibility
// label, not a permission grant.
type ProjectAssignment struct {
	ID         uuid.UUID
	ProjectID  uuid.UUID
	UserID     uuid.UUID
	AssignedBy *uuid.UUID
	AssignedAt time.Time
}

// CreateProjectInput is Service.Create's input.
// documentation/07-api-specification.md §3, `POST /projects`.
type CreateProjectInput struct {
	Name          string
	Description   *string
	RepositoryURL *string
}

// UpdateProjectInput is Service.Update's input — nil fields are left
// unchanged (documentation/07-api-specification.md §3, `PATCH /projects/{id}`
// is a partial update).
type UpdateProjectInput struct {
	Name        *string
	Description *string
}

// RepositoryInput is Service.AttachRepository's input.
// documentation/07-api-specification.md §3, `POST /projects/{id}/repository`.
type RepositoryInput struct {
	URL string
}

// TargetInput is Service.RegisterTarget's input.
// documentation/07-api-specification.md §4, `POST /projects/{id}/targets`.
type TargetInput struct {
	Target string
}

// AttestationInput is Service.AttestTarget's input.
// documentation/07-api-specification.md §4, `POST /targets/{id}/attest`.
type AttestationInput struct {
	AttestationTextVersion string
	Accepted               bool
	SourceIP               string
}
