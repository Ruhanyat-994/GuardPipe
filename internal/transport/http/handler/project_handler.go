package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/project"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/dto"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/middleware"
)

// defaultPageSize/maxPageSize/defaultPage match
// documentation/07-api-specification.md §1.5.
const (
	defaultPage     = 1
	defaultPageSize = 25
	maxPageSize     = 100
)

// ProjectHandler implements the project and pentest-target endpoints in
// documentation/07-api-specification.md §3-4.
type ProjectHandler struct {
	svc       project.Service
	validator *validate.Validator
}

func NewProjectHandler(svc project.Service, validator *validate.Validator) *ProjectHandler {
	return &ProjectHandler{svc: svc, validator: validator}
}

func (h *ProjectHandler) Create(c *gin.Context) {
	var req dto.CreateProjectRequest
	if !h.bindAndValidate(c, &req) {
		return
	}

	actor, ok := requireActor(c)
	if !ok {
		return
	}
	detail, err := h.svc.Create(c.Request.Context(), actor, req.ToInput())
	if err != nil {
		c.Error(err)
		return
	}

	c.Header("Location", "/api/v1/projects/"+detail.ID.String())
	c.JSON(http.StatusCreated, dto.FromProjectDetail(detail))
}

func (h *ProjectHandler) List(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	page := parsePage(c)

	details, total, err := h.svc.List(c.Request.Context(), actor, project.Page{Page: page.Page, PageSize: page.PageSize})
	if err != nil {
		c.Error(err)
		return
	}

	items := make([]dto.ProjectResponse, len(details))
	for i, d := range details {
		items[i] = dto.FromProjectDetail(&d)
	}
	c.JSON(http.StatusOK, dto.ProjectListResponse{Data: items, Pagination: dto.NewPagination(page.Page, page.PageSize, total)})
}

func (h *ProjectHandler) Get(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	projectID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}

	detail, err := h.svc.Get(c.Request.Context(), actor, projectID)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromProjectDetail(detail))
}

func (h *ProjectHandler) Update(c *gin.Context) {
	var req dto.UpdateProjectRequest
	if !h.bindAndValidate(c, &req) {
		return
	}
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	projectID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}

	detail, err := h.svc.Update(c.Request.Context(), actor, projectID, req.ToInput())
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromProjectDetail(detail))
}

func (h *ProjectHandler) Archive(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	projectID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Archive(c.Request.Context(), actor, projectID); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ProjectHandler) AttachRepository(c *gin.Context) {
	var req dto.AttachRepositoryRequest
	if !h.bindAndValidate(c, &req) {
		return
	}
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	projectID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}

	repository, err := h.svc.AttachRepository(c.Request.Context(), actor, projectID, project.RepositoryInput{URL: req.RepositoryURL})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromRepository(repository))
}

func (h *ProjectHandler) SetCredential(c *gin.Context) {
	var req dto.SetCredentialRequest
	if !h.bindAndValidate(c, &req) {
		return
	}
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	projectID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}

	info, err := h.svc.SetCredential(c.Request.Context(), actor, projectID, req.Kind, req.Token)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromCredentialInfo(info))
}

func (h *ProjectHandler) RemoveCredential(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	projectID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.RemoveCredential(c.Request.Context(), actor, projectID); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ProjectHandler) ListTargets(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	projectID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}

	targets, err := h.svc.ListTargets(c.Request.Context(), actor, projectID)
	if err != nil {
		c.Error(err)
		return
	}
	items := make([]dto.TargetResponse, len(targets))
	for i, t := range targets {
		items[i] = dto.FromTarget(&t)
	}
	c.JSON(http.StatusOK, dto.TargetListResponse{Data: items})
}

func (h *ProjectHandler) RegisterTarget(c *gin.Context) {
	var req dto.RegisterTargetRequest
	if !h.bindAndValidate(c, &req) {
		return
	}
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	projectID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}

	target, err := h.svc.RegisterTarget(c.Request.Context(), actor, projectID, project.TargetInput{Target: req.Target})
	if err != nil {
		c.Error(err)
		return
	}
	c.Header("Location", "/api/v1/targets/"+target.ID.String())
	c.JSON(http.StatusCreated, dto.FromTarget(target))
}

func (h *ProjectHandler) AttestTarget(c *gin.Context) {
	var req dto.AttestTargetRequest
	if !h.bindAndValidate(c, &req) {
		return
	}
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	targetID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}

	target, attestation, err := h.svc.AttestTarget(c.Request.Context(), actor, targetID, req.ToInput(c.ClientIP()))
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, dto.FromAttestation(target, attestation))
}

func (h *ProjectHandler) RevokeTarget(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	targetID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.RevokeTarget(c.Request.Context(), actor, targetID); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

// --- shared handler-layer helpers ---

func (h *ProjectHandler) bindAndValidate(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		c.Error(apperrors.Validation("project.invalid_body", "request body could not be parsed", nil))
		return false
	}
	if fieldErrs := h.validator.Struct(req); len(fieldErrs) > 0 {
		c.Error(apperrors.Validation("project.invalid_input", "one or more fields are invalid", toAppFieldErrors(fieldErrs)))
		return false
	}
	return true
}

func requireActor(c *gin.Context) (domain.Actor, bool) {
	actor, ok := middleware.ActorFromContext(c)
	if !ok {
		c.Error(apperrors.Internal(errors.New("project handler reached without an authenticated actor")))
		return domain.Actor{}, false
	}
	return actor, true
}

func requirePathUUID(c *gin.Context, param string) (uuid.UUID, bool) {
	v, err := uuid.Parse(c.Param(param))
	if err != nil {
		c.Error(apperrors.Validation("project.invalid_id", "path parameter is not a valid UUID", nil))
		return uuid.UUID{}, false
	}
	return v, true
}

// parsedPage is page/page_size after applying
// documentation/07-api-specification.md §1.5's defaults and clamp.
type parsedPage struct {
	Page     int
	PageSize int
}

func parsePage(c *gin.Context) parsedPage {
	page := defaultPage
	if v, err := strconv.Atoi(c.Query("page")); err == nil && v > 0 {
		page = v
	}
	pageSize := defaultPageSize
	if v, err := strconv.Atoi(c.Query("page_size")); err == nil && v > 0 {
		pageSize = v
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return parsedPage{Page: page, PageSize: pageSize}
}
