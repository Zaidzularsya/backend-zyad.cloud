package handler

import (
	"context"
	"net/http"

	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type SelfOrganizationService interface {
	Get(context.Context, string) (dto.OrganizationResponse, error)
	Update(
		context.Context,
		string,
		string,
		string,
		dto.UpdateCurrentOrganizationRequest,
	) (dto.OrganizationResponse, error)
	ListMembers(
		context.Context,
		string,
		dto.MembershipListQuery,
	) ([]dto.MembershipResponse, dto.PaginationMeta, error)
	Invite(
		context.Context,
		string,
		string,
		dto.InviteMemberRequest,
	) (dto.InvitationResponse, error)
	ChangeMemberStatus(
		context.Context,
		string,
		string,
		string,
		dto.UpdateMembershipStatusRequest,
	) (dto.MembershipResponse, error)
	RemoveMember(
		context.Context,
		string,
		string,
		string,
		string,
	) (dto.MembershipResponse, error)
}

type SelfHandler struct {
	service SelfOrganizationService
	checker permissionmiddleware.OrganizationPermissionChecker
}

func NewSelfHandler(
	service SelfOrganizationService,
	checker permissionmiddleware.OrganizationPermissionChecker,
) *SelfHandler {
	return &SelfHandler{service: service, checker: checker}
}

func (h *SelfHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/organization")
	group.Use(
		middleware.RequireActiveTenant(),
		middleware.RequireCustomerTenant(),
	)
	group.GET(
		"",
		permissionmiddleware.RequireOrganization(h.checker, "organization.read"),
		h.Get,
	)
	group.PATCH(
		"",
		permissionmiddleware.RequireOrganization(h.checker, "organization.update"),
		h.Update,
	)
	group.GET(
		"/members",
		permissionmiddleware.RequireOrganization(h.checker, "organization.member.read"),
		h.ListMembers,
	)
	group.POST(
		"/invitations",
		permissionmiddleware.RequireOrganization(h.checker, "organization.member.manage"),
		h.Invite,
	)
	group.PATCH(
		"/members/:id/status",
		permissionmiddleware.RequireOrganization(h.checker, "organization.member.manage"),
		h.ChangeMemberStatus,
	)
	group.DELETE(
		"/members/:id",
		permissionmiddleware.RequireOrganization(h.checker, "organization.member.manage"),
		h.RemoveMember,
	)
}

func (h *SelfHandler) Get(c *gin.Context) {
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	organization, err := h.service.Get(
		c.Request.Context(),
		tenantContext.OrganizationID(),
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "organization retrieved successfully", organization)
}

func (h *SelfHandler) Update(c *gin.Context) {
	var request dto.UpdateCurrentOrganizationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	organization, err := h.service.Update(
		c.Request.Context(),
		tenantContext.OrganizationID(),
		tenantContext.MembershipID(),
		permissionmiddleware.UserID(c),
		request,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "organization updated successfully", organization)
}

func (h *SelfHandler) ListMembers(c *gin.Context) {
	var query dto.MembershipListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	items, meta, err := h.service.ListMembers(
		c.Request.Context(),
		tenantContext.OrganizationID(),
		query,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "organization members retrieved successfully", items, meta)
}

func (h *SelfHandler) Invite(c *gin.Context) {
	var request dto.InviteMemberRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	invitation, err := h.service.Invite(
		c.Request.Context(),
		tenantContext.OrganizationID(),
		permissionmiddleware.UserID(c),
		request,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "member invited successfully", invitation)
}

func (h *SelfHandler) ChangeMemberStatus(c *gin.Context) {
	var request dto.UpdateMembershipStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	membership, err := h.service.ChangeMemberStatus(
		c.Request.Context(),
		tenantContext.OrganizationID(),
		c.Param("id"),
		permissionmiddleware.UserID(c),
		request,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "membership status updated successfully", membership)
}

func (h *SelfHandler) RemoveMember(c *gin.Context) {
	var request dto.RemoveMembershipRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	membership, err := h.service.RemoveMember(
		c.Request.Context(),
		tenantContext.OrganizationID(),
		c.Param("id"),
		permissionmiddleware.UserID(c),
		request.Reason,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "member removed successfully", membership)
}
