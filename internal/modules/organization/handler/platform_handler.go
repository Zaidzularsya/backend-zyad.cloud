package handler

import (
	"context"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type PlatformOrganizationService interface {
	List(
		context.Context,
		dto.OrganizationListQuery,
	) ([]dto.OrganizationResponse, dto.PaginationMeta, error)
	Get(context.Context, string) (dto.PlatformOrganizationDetailResponse, error)
	Create(context.Context, dto.CreateOrganizationRequest, string) (dto.OrganizationResponse, error)
	Update(context.Context, string, dto.UpdateOrganizationRequest) (dto.OrganizationResponse, error)
	ChangeStatus(
		context.Context,
		string,
		dto.UpdateOrganizationStatusRequest,
		string,
	) (dto.OrganizationResponse, error)
	Provision(
		context.Context,
		string,
		string,
	) (dto.PlatformOrganizationDetailResponse, error)
}

type PlatformHandler struct {
	service PlatformOrganizationService
	checker permissionmiddleware.PermissionChecker
}

func NewPlatformHandler(
	service PlatformOrganizationService,
	checker permissionmiddleware.PermissionChecker,
) *PlatformHandler {
	return &PlatformHandler{service: service, checker: checker}
}

func (h *PlatformHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/platform/organizations")
	group.Use(middleware.RequirePlatformTenant())
	group.GET(
		"",
		permissionmiddleware.Require(h.checker, "platform.organization.read"),
		h.List,
	)
	group.POST(
		"",
		permissionmiddleware.Require(h.checker, "platform.organization.manage"),
		h.Create,
	)
	group.GET(
		"/:id",
		permissionmiddleware.Require(h.checker, "platform.organization.read"),
		h.Get,
	)
	group.PATCH(
		"/:id",
		permissionmiddleware.Require(h.checker, "platform.organization.manage"),
		h.Update,
	)
	group.PATCH(
		"/:id/status",
		permissionmiddleware.Require(h.checker, "platform.organization.suspend"),
		h.ChangeStatus,
	)
	group.POST(
		"/:id/provision",
		permissionmiddleware.Require(h.checker, "platform.organization.provision"),
		h.Provision,
	)
}

func (h *PlatformHandler) List(c *gin.Context) {
	var query dto.OrganizationListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	items, meta, err := h.service.List(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "organizations retrieved successfully", items, meta)
}

func (h *PlatformHandler) Get(c *gin.Context) {
	organization, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "organization retrieved successfully", organization)
}

func (h *PlatformHandler) Create(c *gin.Context) {
	var request dto.CreateOrganizationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	organization, err := h.service.Create(
		c.Request.Context(),
		request,
		permissionmiddleware.UserID(c),
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "organization created successfully", organization)
}

func (h *PlatformHandler) Update(c *gin.Context) {
	var request dto.UpdateOrganizationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	organization, err := h.service.Update(
		c.Request.Context(),
		c.Param("id"),
		request,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "organization updated successfully", organization)
}

func (h *PlatformHandler) ChangeStatus(c *gin.Context) {
	var request dto.UpdateOrganizationStatusRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	organization, err := h.service.ChangeStatus(
		c.Request.Context(),
		c.Param("id"),
		request,
		permissionmiddleware.UserID(c),
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "organization status updated successfully", organization)
}

func (h *PlatformHandler) Provision(c *gin.Context) {
	organization, err := h.service.Provision(
		c.Request.Context(),
		c.Param("id"),
		permissionmiddleware.UserID(c),
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "organization provisioned successfully", organization)
}

func validationHandlerError(err error) error {
	return coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity)
}
