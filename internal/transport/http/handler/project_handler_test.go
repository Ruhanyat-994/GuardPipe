package handler_test

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/id"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/handler"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/middleware"
)

// fakeProjectService is a hand-written fake — these tests are about the
// HTTP layer (binding, validation, status codes, error mapping), the
// business logic itself is covered by internal/modules/project/service_test.go.
type fakeProjectService struct {
	detail  *project.ProjectDetail
	details []project.ProjectDetail
	total   int
	err     error

	repo *project.Repository
	cred *project.CredentialInfo

	targets     []project.Target
	target      *project.Target
	attestation *project.TargetAttestation

	documents []project.Document
	document  *project.Document
}

func (f *fakeProjectService) Create(context.Context, domain.Actor, project.CreateProjectInput) (*project.ProjectDetail, error) {
	return f.detail, f.err
}
func (f *fakeProjectService) List(context.Context, domain.Actor, project.Page) ([]project.ProjectDetail, int, error) {
	return f.details, f.total, f.err
}
func (f *fakeProjectService) Get(context.Context, domain.Actor, uuid.UUID) (*project.ProjectDetail, error) {
	return f.detail, f.err
}
func (f *fakeProjectService) Update(context.Context, domain.Actor, uuid.UUID, project.UpdateProjectInput) (*project.ProjectDetail, error) {
	return f.detail, f.err
}
func (f *fakeProjectService) Archive(context.Context, domain.Actor, uuid.UUID) error { return f.err }
func (f *fakeProjectService) AttachRepository(context.Context, domain.Actor, uuid.UUID, project.RepositoryInput) (*project.Repository, error) {
	return f.repo, f.err
}
func (f *fakeProjectService) SetCredential(context.Context, domain.Actor, uuid.UUID, string, string) (*project.CredentialInfo, error) {
	return f.cred, f.err
}
func (f *fakeProjectService) RemoveCredential(context.Context, domain.Actor, uuid.UUID) error {
	return f.err
}
func (f *fakeProjectService) ListTargets(context.Context, domain.Actor, uuid.UUID) ([]project.Target, error) {
	return f.targets, f.err
}
func (f *fakeProjectService) RegisterTarget(context.Context, domain.Actor, uuid.UUID, project.TargetInput) (*project.Target, error) {
	return f.target, f.err
}
func (f *fakeProjectService) AttestTarget(context.Context, domain.Actor, uuid.UUID, project.AttestationInput) (*project.Target, *project.TargetAttestation, error) {
	return f.target, f.attestation, f.err
}
func (f *fakeProjectService) RevokeTarget(context.Context, domain.Actor, uuid.UUID) error {
	return f.err
}
func (f *fakeProjectService) AdminRevokeTarget(context.Context, uuid.UUID) error {
	return f.err
}
func (f *fakeProjectService) GetCloneInfo(context.Context, uuid.UUID) (string, string, string, error) {
	return "", "", "", f.err
}
func (f *fakeProjectService) GetAttestedTarget(context.Context, uuid.UUID) (*project.Target, error) {
	return nil, f.err
}
func (f *fakeProjectService) MarkCredentialInvalid(context.Context, uuid.UUID, string) error {
	return f.err
}
func (f *fakeProjectService) ListDocuments(context.Context, domain.Actor, uuid.UUID) ([]project.Document, error) {
	return f.documents, f.err
}
func (f *fakeProjectService) UploadDocument(context.Context, domain.Actor, uuid.UUID, string, string, []byte) (*project.Document, error) {
	return f.document, f.err
}
func (f *fakeProjectService) ImportDocumentFromURL(context.Context, domain.Actor, uuid.UUID, string) (*project.Document, error) {
	return f.document, f.err
}
func (f *fakeProjectService) DeleteDocument(context.Context, domain.Actor, uuid.UUID) error {
	return f.err
}
func (f *fakeProjectService) GetDocuments(context.Context, uuid.UUID) ([]project.Document, error) {
	return f.documents, f.err
}
func (f *fakeProjectService) GetOrgID(context.Context, uuid.UUID) (uuid.UUID, error) {
	return uuid.UUID{}, f.err
}
func (f *fakeProjectService) AssignProject(context.Context, domain.Actor, uuid.UUID, uuid.UUID) (*project.ProjectAssignment, error) {
	return nil, f.err
}
func (f *fakeProjectService) UnassignProject(context.Context, domain.Actor, uuid.UUID, uuid.UUID) error {
	return f.err
}
func (f *fakeProjectService) ListAssignments(context.Context, domain.Actor, uuid.UUID) ([]project.ProjectAssignment, error) {
	return nil, f.err
}
func (f *fakeProjectService) ListAssignmentsForOrg(context.Context, domain.Actor) ([]project.ProjectAssignment, error) {
	return nil, f.err
}

func newProjectRouter(svc project.Service) *gin.Engine {
	r := gin.New()
	r.Use(middleware.ErrorMapper())
	r.Use(func(c *gin.Context) {
		c.Set("actor", domain.Actor{UserID: id.New(), OrgID: id.New(), Role: domain.RoleMember})
		c.Next()
	})

	h := handler.NewProjectHandler(svc, validate.New())
	r.GET("/projects", h.List)
	r.POST("/projects", h.Create)
	r.GET("/projects/:id", h.Get)
	r.PATCH("/projects/:id", h.Update)
	r.DELETE("/projects/:id", h.Archive)
	r.POST("/projects/:id/repository", h.AttachRepository)
	r.PUT("/projects/:id/credential", h.SetCredential)
	r.GET("/projects/:id/targets", h.ListTargets)
	r.POST("/projects/:id/targets", h.RegisterTarget)
	r.POST("/targets/:id/attest", h.AttestTarget)
	r.DELETE("/targets/:id", h.RevokeTarget)
	r.GET("/projects/:id/documents", h.ListDocuments)
	r.POST("/projects/:id/documents", h.UploadDocument)
	r.POST("/projects/:id/documents/import", h.ImportDocument)
	r.DELETE("/documents/:id", h.DeleteDocument)
	return r
}

func sampleDetail() *project.ProjectDetail {
	name := "Payments API"
	return &project.ProjectDetail{
		Project: project.Project{ID: id.New(), Name: name, Status: project.StatusActive, CreatedAt: time.Now()},
	}
}

func TestProjectCreate_ValidRequestReturns201(t *testing.T) {
	svc := &fakeProjectService{detail: sampleDetail()}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/projects", map[string]string{"name": "Payments API"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body: %s", rec.Code, rec.Body.String())
	}
}

func TestProjectCreate_MissingName_Returns400(t *testing.T) {
	svc := &fakeProjectService{}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/projects", map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestProjectCreate_InvalidRepositoryURL_Returns400(t *testing.T) {
	// Near-miss: "repository_url" isn't a URL at all — the validator tag
	// must catch this before it ever reaches the service.
	svc := &fakeProjectService{}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/projects", map[string]string{"name": "X", "repository_url": "not a url"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestProjectGet_NotFound_Returns404(t *testing.T) {
	svc := &fakeProjectService{err: apperrors.NotFound("project.not_found", "project not found")}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/projects/"+id.New().String(), nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body: %s", rec.Code, rec.Body.String())
	}
}

func TestProjectGet_InvalidUUID_Returns400(t *testing.T) {
	svc := &fakeProjectService{}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/projects/not-a-uuid", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestProjectArchive_Returns204(t *testing.T) {
	svc := &fakeProjectService{}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodDelete, "/projects/"+id.New().String(), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body: %s", rec.Code, rec.Body.String())
	}
}

func TestSetCredential_NeverEchoesToken(t *testing.T) {
	svc := &fakeProjectService{cred: &project.CredentialInfo{HasCredential: true, Hint: "ghp_••••3f9a"}}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodPut, "/projects/"+id.New().String()+"/credential",
		map[string]string{"kind": "github_pat", "token": "ghp_supersecrettoken1234"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); strings.Contains(got, "supersecrettoken") {
		t.Fatalf("response leaked the raw token: %s", got)
	}
}

func TestSetCredential_InvalidKind_Returns400(t *testing.T) {
	svc := &fakeProjectService{}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodPut, "/projects/"+id.New().String()+"/credential",
		map[string]string{"kind": "ssh_key", "token": "whatever"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterTarget_BlockedAddress_Returns422(t *testing.T) {
	svc := &fakeProjectService{err: apperrors.Unprocessable("target.blocked_address", "192.168.1.50 is blocked")}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/projects/"+id.New().String()+"/targets", map[string]string{"target": "internal.acme.example"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body: %s", rec.Code, rec.Body.String())
	}
}

func TestAttestTarget_Returns200(t *testing.T) {
	target := &project.Target{ID: id.New(), Status: project.TargetAttested}
	svc := &fakeProjectService{target: target, attestation: &project.TargetAttestation{AttestedAt: time.Now(), AttestedByID: id.New(), AttestedByName: "Nadia R."}}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/targets/"+id.New().String()+"/attest",
		map[string]any{"attestation_text_version": "v1", "accepted": true, "statement": "I confirm ownership."})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

func TestListDocuments_Returns200(t *testing.T) {
	svc := &fakeProjectService{documents: []project.Document{{ID: id.New(), Filename: "srs.md"}}}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodGet, "/projects/"+id.New().String()+"/documents", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", rec.Code, rec.Body.String())
	}
}

// doMultipart builds a single-file "file" multipart/form-data request —
// UploadDocument is the one handler in this package that isn't JSON, so it
// needs its own request builder alongside doJSON.
func doMultipart(t *testing.T, r *gin.Engine, path, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestUploadDocument_Returns201(t *testing.T) {
	svc := &fakeProjectService{document: &project.Document{ID: id.New(), Filename: "srs.md", SizeBytes: 5}}
	r := newProjectRouter(svc)

	rec := doMultipart(t, r, "/projects/"+id.New().String()+"/documents", "srs.md", []byte("# SRS"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadDocument_MissingFileField_Returns400(t *testing.T) {
	svc := &fakeProjectService{}
	r := newProjectRouter(svc)

	// A well-formed multipart request with no "file" field at all.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/projects/"+id.New().String()+"/documents", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestUploadDocument_UnsupportedType_Returns400(t *testing.T) {
	svc := &fakeProjectService{err: apperrors.Validation("document.unsupported_type", "filename must end in .md, .txt, .adoc, or .rst", nil)}
	r := newProjectRouter(svc)

	rec := doMultipart(t, r, "/projects/"+id.New().String()+"/documents", "srs.pdf", []byte("%PDF-1.4"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestImportDocument_Returns201(t *testing.T) {
	svc := &fakeProjectService{document: &project.Document{ID: id.New(), Filename: "google-doc-1AbC-xyz.txt", SizeBytes: 20}}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/projects/"+id.New().String()+"/documents/import",
		map[string]string{"url": "https://docs.google.com/document/d/1AbC-xyz_123/edit"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body: %s", rec.Code, rec.Body.String())
	}
}

func TestImportDocument_MissingURL_Returns400(t *testing.T) {
	svc := &fakeProjectService{}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/projects/"+id.New().String()+"/documents/import", map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestImportDocument_ServiceRejectsInvalidURL_Returns400(t *testing.T) {
	svc := &fakeProjectService{err: apperrors.Validation("document.invalid_google_url", "must be a docs.google.com or drive.google.com link", nil)}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/projects/"+id.New().String()+"/documents/import",
		map[string]string{"url": "https://example.com/not-google"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", rec.Code, rec.Body.String())
	}
}

func TestImportDocument_ServiceRejectsPrivateLink_Returns422(t *testing.T) {
	svc := &fakeProjectService{err: apperrors.Unprocessable("document.not_publicly_accessible", `this document isn't shared publicly`)}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodPost, "/projects/"+id.New().String()+"/documents/import",
		map[string]string{"url": "https://docs.google.com/document/d/1AbC-xyz_123/edit"})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteDocument_Returns204(t *testing.T) {
	svc := &fakeProjectService{}
	r := newProjectRouter(svc)

	rec := doJSON(t, r, http.MethodDelete, "/documents/"+id.New().String(), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body: %s", rec.Code, rec.Body.String())
	}
}
