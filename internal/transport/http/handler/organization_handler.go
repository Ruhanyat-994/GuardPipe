package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Ruhanyat-994/GuardPipe/internal/domain"
	"github.com/Ruhanyat-994/GuardPipe/internal/modules/organization"
	apperrors "github.com/Ruhanyat-994/GuardPipe/internal/platform/errors"
	"github.com/Ruhanyat-994/GuardPipe/internal/platform/validate"
	"github.com/Ruhanyat-994/GuardPipe/internal/transport/http/dto"
)

// OrganizationHandler implements BUILD_GUIDE.md Phase 15's org-membership
// endpoints — members, invites, and the org-switcher list. SwitchOrg itself
// lives on AuthHandler (see that handler's own doc comment on orgSvc) since
// it needs Login/Refresh's exact refresh-cookie machinery.
type OrganizationHandler struct {
	svc       organization.Service
	validator *validate.Validator
}

func NewOrganizationHandler(svc organization.Service, validator *validate.Validator) *OrganizationHandler {
	return &OrganizationHandler{svc: svc, validator: validator}
}

// ListMine is `GET /organizations` — the org-switcher's own list, every
// organisation the caller can currently switch into.
func (h *OrganizationHandler) ListMine(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	orgs, err := h.svc.ListMemberOrgs(c.Request.Context(), actor)
	if err != nil {
		c.Error(err)
		return
	}
	items := make([]dto.MemberOrgResponse, len(orgs))
	for i, o := range orgs {
		items[i] = dto.FromMemberOrgSummary(o)
	}
	c.JSON(http.StatusOK, dto.MemberOrgListResponse{Data: items})
}

func (h *OrganizationHandler) ListMembers(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	orgID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	members, err := h.svc.ListMembers(c.Request.Context(), actor, orgID)
	if err != nil {
		c.Error(err)
		return
	}
	items := make([]dto.MemberSummaryResponse, len(members))
	for i, m := range members {
		items[i] = dto.FromMemberSummary(m)
	}
	c.JSON(http.StatusOK, dto.MemberListResponse{Data: items})
}

func (h *OrganizationHandler) UpdateMemberRole(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	orgID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	userID, ok := requirePathUUID(c, "userId")
	if !ok {
		return
	}
	var req dto.UpdateMemberRoleRequest
	if !h.bindAndValidate(c, &req) {
		return
	}
	if err := h.svc.UpdateMemberRole(c.Request.Context(), actor, orgID, userID, domain.Role(req.Role)); err != nil {
		c.Error(err)
		return
	}
	h.ListMembers(c)
}

func (h *OrganizationHandler) RemoveMember(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	orgID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	userID, ok := requirePathUUID(c, "userId")
	if !ok {
		return
	}
	if err := h.svc.RemoveMember(c.Request.Context(), actor, orgID, userID); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *OrganizationHandler) CreateInvite(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	orgID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	var req dto.CreateInviteRequest
	if !h.bindAndValidate(c, &req) {
		return
	}
	created, err := h.svc.CreateInvite(c.Request.Context(), actor, orgID, organization.InviteInput{Email: req.Email, Role: domain.Role(req.Role)})
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, dto.FromCreatedInvite(*created))
}

func (h *OrganizationHandler) ListInvites(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	orgID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	invites, err := h.svc.ListInvites(c.Request.Context(), actor, orgID)
	if err != nil {
		c.Error(err)
		return
	}
	items := make([]dto.InviteResponse, len(invites))
	for i, inv := range invites {
		items[i] = dto.FromInvite(inv)
	}
	c.JSON(http.StatusOK, dto.InviteListResponse{Data: items})
}

func (h *OrganizationHandler) RevokeInvite(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	orgID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	inviteID, ok := requirePathUUID(c, "inviteId")
	if !ok {
		return
	}
	if err := h.svc.RevokeInvite(c.Request.Context(), actor, orgID, inviteID); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

// AcceptInvite is `POST /invites/{id}/accept` — requires the caller to
// already be authenticated as the invited email's account (see
// organization.Service.AcceptInvite's own doc comment for the two-call
// register-or-login-then-accept flow this implies, and for why `id` here
// accepts either the invite's UUID — the live-notification flow, the
// primary one — or its raw token, the copy-a-link fallback).
func (h *OrganizationHandler) AcceptInvite(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	identifier := c.Param("id")
	if identifier == "" {
		c.Error(apperrors.Validation("organization.invalid_input", "token is required", nil))
		return
	}
	membership, err := h.svc.AcceptInvite(c.Request.Context(), actor, identifier)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"org_id": membership.OrgID.String(), "role": string(membership.Role)})
}

// ListMyInvites is `GET /invites/mine` — the live notification feed
// NotificationPanel.tsx polls (2026-09-02 follow-up to Phase 15): every
// still-pending invite addressed to the caller's own account email.
func (h *OrganizationHandler) ListMyInvites(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	invites, err := h.svc.ListMyInvites(c.Request.Context(), actor)
	if err != nil {
		c.Error(err)
		return
	}
	items := make([]dto.PendingInviteResponse, len(invites))
	for i, inv := range invites {
		items[i] = dto.FromPendingInvite(inv)
	}
	c.JSON(http.StatusOK, dto.PendingInviteListResponse{Data: items})
}

// DeclineInvite is `POST /invites/{id}/decline` — the invitee's own
// counterpart to an org admin's RevokeInvite.
func (h *OrganizationHandler) DeclineInvite(c *gin.Context) {
	actor, ok := requireActor(c)
	if !ok {
		return
	}
	inviteID, ok := requirePathUUID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.DeclineInvite(c.Request.Context(), actor, inviteID); err != nil {
		c.Error(err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *OrganizationHandler) bindAndValidate(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		c.Error(apperrors.Validation("organization.invalid_body", "request body could not be parsed", nil))
		return false
	}
	if fieldErrs := h.validator.Struct(req); len(fieldErrs) > 0 {
		c.Error(apperrors.Validation("organization.invalid_input", "one or more fields are invalid", toAppFieldErrors(fieldErrs)))
		return false
	}
	return true
}
