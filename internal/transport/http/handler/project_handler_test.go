package handler_test

import (
	"context"
	"net/http"
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
