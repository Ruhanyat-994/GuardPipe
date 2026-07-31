package dto

import (
	"time"

	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
)

// --- projects — documentation/07-api-specification.md §3 ---

// CreateProjectRequest matches `POST /projects`.
type CreateProjectRequest struct {
	Name          string  `json:"name" validate:"required,min=1,max=120"`
	Description   *string `json:"description" validate:"omitempty,max=2000"`
	RepositoryURL *string `json:"repository_url" validate:"omitempty,url"`
}

func (r CreateProjectRequest) ToInput() project.CreateProjectInput {
	return project.CreateProjectInput{Name: r.Name, Description: r.Description, RepositoryURL: r.RepositoryURL}
}

// UpdateProjectRequest matches `PATCH /projects/{id}` — a partial update,
// so every field is optional and a nil field is left unchanged.
type UpdateProjectRequest struct {
	Name        *string `json:"name" validate:"omitempty,min=1,max=120"`
	Description *string `json:"description" validate:"omitempty,max=2000"`
}

func (r UpdateProjectRequest) ToInput() project.UpdateProjectInput {
	return project.UpdateProjectInput{Name: r.Name, Description: r.Description}
}

// AttachRepositoryRequest matches `POST /projects/{id}/repository`.
type AttachRepositoryRequest struct {
	RepositoryURL string `json:"repository_url" validate:"required,url"`
}

// SetCredentialRequest matches `PUT /projects/{id}/credential`.
type SetCredentialRequest struct {
	Kind  string `json:"kind" validate:"required,oneof=github_pat"`
	Token string `json:"token" validate:"required,min=1"`
}

// RepositoryResponse matches the nested "repository" object in
// documentation/07-api-specification.md §3's `POST /projects` example.
type RepositoryResponse struct {
	ID            string `json:"id"`
	Provider      string `json:"provider"`
	URL           string `json:"url"`
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch"`
	IsPrivate     bool   `json:"is_private"`
	SizeKB        *int64 `json:"size_kb"`
}

func FromRepository(r *project.Repository) *RepositoryResponse {
	if r == nil {
		return nil
	}
	return &RepositoryResponse{
		ID: r.ID.String(), Provider: r.Provider, URL: r.URL, Owner: r.Owner, Name: r.Name,
		DefaultBranch: r.DefaultBranch, IsPrivate: r.IsPrivate, SizeKB: r.SizeKB,
	}
}

// ProjectResponse matches documentation/07-api-specification.md §3's
// `POST /projects` response shape, reused for every other project
// endpoint that returns a single project.
type ProjectResponse struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Description   *string             `json:"description"`
	Status        string              `json:"status"`
	Repository    *RepositoryResponse `json:"repository"`
	HasCredential bool                `json:"has_credential"`
	// LatestScan is always null until the `scans` table exists
	// (BUILD_GUIDE.md Phase 6) — present per the "nulls are present, not
	// omitted" convention (documentation/07-api-specification.md §1).
	LatestScan any       `json:"latest_scan"`
	CreatedAt  time.Time `json:"created_at"`
}

func FromProjectDetail(d *project.ProjectDetail) ProjectResponse {
	return ProjectResponse{
		ID:            d.ID.String(),
		Name:          d.Name,
		Description:   d.Description,
		Status:        string(d.Status),
		Repository:    FromRepository(d.Repository),
		HasCredential: d.HasCredential,
		LatestScan:    nil,
		CreatedAt:     d.CreatedAt,
	}
}

// Pagination matches documentation/07-api-specification.md §1.5's
// collection envelope.
type Pagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

func NewPagination(page, pageSize, total int) Pagination {
	totalPages := max((total+pageSize-1)/pageSize, 1)
	return Pagination{Page: page, PageSize: pageSize, Total: total, TotalPages: totalPages}
}

// ProjectListResponse matches `GET /projects`.
type ProjectListResponse struct {
	Data       []ProjectResponse `json:"data"`
	Pagination Pagination        `json:"pagination"`
}

// CredentialResponse matches `PUT /projects/{id}/credential`'s response —
// the token itself is never present (FR-PRJ-004).
type CredentialResponse struct {
	HasCredential bool       `json:"has_credential"`
	Hint          string     `json:"hint"`
	UpdatedAt     *time.Time `json:"updated_at"`
}

func FromCredentialInfo(c *project.CredentialInfo) CredentialResponse {
	return CredentialResponse{HasCredential: c.HasCredential, Hint: c.Hint, UpdatedAt: c.UpdatedAt}
}

// --- pentest targets — documentation/07-api-specification.md §4 ---

// RegisterTargetRequest matches `POST /projects/{id}/targets`.
type RegisterTargetRequest struct {
	Target string `json:"target" validate:"required"`
}

// AttestTargetRequest matches `POST /targets/{id}/attest`. Statement is
// accepted and validated as non-empty (the user must see and agree to
// specific wording) but is not stored verbatim — `attestation_text_version`
// identifies which wording it was (documentation/06-database-design.md
// §4.8).
type AttestTargetRequest struct {
	AttestationTextVersion string `json:"attestation_text_version" validate:"required"`
	Accepted               bool   `json:"accepted"`
	Statement              string `json:"statement" validate:"required"`
}

func (r AttestTargetRequest) ToInput(sourceIP string) project.AttestationInput {
	return project.AttestationInput{AttestationTextVersion: r.AttestationTextVersion, Accepted: r.Accepted, SourceIP: sourceIP}
}

// TargetResponse matches `POST /projects/{id}/targets`'s 201 response.
type TargetResponse struct {
	ID             string    `json:"id"`
	Target         string    `json:"target"`
	NormalizedHost string    `json:"normalized_host"`
	PinnedIPs      []string  `json:"pinned_ips"`
	Status         string    `json:"status"`
	LastResolvedAt time.Time `json:"last_resolved_at"`
}

func FromTarget(t *project.Target) TargetResponse {
	ips := make([]string, len(t.PinnedIPs))
	for i, addr := range t.PinnedIPs {
		ips[i] = addr.String()
	}
	return TargetResponse{
		ID: t.ID.String(), Target: t.TargetInput, NormalizedHost: t.NormalizedHost,
		PinnedIPs: ips, Status: string(t.Status), LastResolvedAt: t.LastResolvedAt,
	}
}

// AttestedByResponse matches the nested "attested_by" object in
// documentation/07-api-specification.md §4.
type AttestedByResponse struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// AttestTargetResponse matches `POST /targets/{id}/attest`'s 200 response.
type AttestTargetResponse struct {
	ID         string             `json:"id"`
	Status     string             `json:"status"`
	AttestedAt time.Time          `json:"attested_at"`
	AttestedBy AttestedByResponse `json:"attested_by"`
}

func FromAttestation(t *project.Target, a *project.TargetAttestation) AttestTargetResponse {
	return AttestTargetResponse{
		ID: t.ID.String(), Status: string(t.Status), AttestedAt: a.AttestedAt,
		AttestedBy: AttestedByResponse{ID: a.AttestedByID.String(), DisplayName: a.AttestedByName},
	}
}

// TargetListResponse matches `GET /projects/{id}/targets`.
type TargetListResponse struct {
	Data []TargetResponse `json:"data"`
}
