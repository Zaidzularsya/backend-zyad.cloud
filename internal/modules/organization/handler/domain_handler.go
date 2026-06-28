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

type OrganizationDomainService interface {
	List(
		context.Context,
		string,
		dto.DomainListQuery,
	) ([]dto.DomainResponse, dto.PaginationMeta, error)
	Create(
		context.Context,
		string,
		dto.CreateDomainRequest,
		string,
	) (dto.DomainChallengeResponse, error)
	Verify(context.Context, string, string, string) (dto.DomainResponse, error)
	Update(
		context.Context,
		string,
		string,
		dto.UpdateDomainRequest,
		string,
	) (dto.DomainResponse, error)
	Delete(context.Context, string, string, string) error
}

type DomainHandler struct {
	service OrganizationDomainService
	checker permissionmiddleware.CombinedPermissionChecker
}

func NewDomainHandler(
	service OrganizationDomainService,
	checker permissionmiddleware.CombinedPermissionChecker,
) *DomainHandler {
	return &DomainHandler{service: service, checker: checker}
}

func (h *DomainHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/organization/domains")
	group.Use(
		middleware.RequireActiveTenant(),
		permissionmiddleware.RequireOrganizationOrGlobal(
			h.checker,
			"organization.domain.manage",
		),
	)
	group.GET("", h.List)
	group.POST("", h.Create)
	group.POST("/:id/verify", h.Verify)
	group.PATCH("/:id", h.Update)
	group.DELETE("/:id", h.Delete)
}

func (h *DomainHandler) List(c *gin.Context) {
	var query dto.DomainListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	items, meta, err := h.service.List(
		c.Request.Context(),
		tenantContext.OrganizationID(),
		query,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "organization domains retrieved successfully", items, meta)
}

func (h *DomainHandler) Create(c *gin.Context) {
	var request dto.CreateDomainRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	challenge, err := h.service.Create(
		c.Request.Context(),
		tenantContext.OrganizationID(),
		request,
		permissionmiddleware.UserID(c),
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "organization domain created successfully", challenge)
}

func (h *DomainHandler) Verify(c *gin.Context) {
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	domain, err := h.service.Verify(
		c.Request.Context(),
		tenantContext.OrganizationID(),
		c.Param("id"),
		permissionmiddleware.UserID(c),
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "organization domain verified successfully", domain)
}

func (h *DomainHandler) Update(c *gin.Context) {
	var request dto.UpdateDomainRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	domain, err := h.service.Update(
		c.Request.Context(),
		tenantContext.OrganizationID(),
		c.Param("id"),
		request,
		permissionmiddleware.UserID(c),
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "primary organization domain updated successfully", domain)
}

func (h *DomainHandler) Delete(c *gin.Context) {
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	if err := h.service.Delete(
		c.Request.Context(),
		tenantContext.OrganizationID(),
		c.Param("id"),
		permissionmiddleware.UserID(c),
	); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "organization domain disabled successfully", gin.H{
		"id": c.Param("id"),
	})
}
