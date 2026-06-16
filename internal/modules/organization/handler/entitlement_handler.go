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

type OrganizationEntitlementService interface {
	ListFeatures(
		context.Context,
		string,
		dto.EntitlementListQuery,
	) ([]dto.EffectiveFeatureResponse, dto.PaginationMeta, error)
	CheckUsage(context.Context, string, dto.UsageQuery) (dto.UsageResponse, error)
	UpsertPlatformOverride(
		context.Context,
		string,
		dto.UpsertEntitlementRequest,
		string,
	) (dto.EntitlementResponse, error)
}

type EntitlementHandler struct {
	service             OrganizationEntitlementService
	checker             permissionmiddleware.OrganizationPermissionChecker
	platformPermChecker permissionmiddleware.PermissionChecker
}

func NewEntitlementHandler(
	service OrganizationEntitlementService,
	checker permissionmiddleware.OrganizationPermissionChecker,
	platformPermChecker permissionmiddleware.PermissionChecker,
) *EntitlementHandler {
	return &EntitlementHandler{
		service:             service,
		checker:             checker,
		platformPermChecker: platformPermChecker,
	}
}

func (h *EntitlementHandler) RegisterRoutes(router *gin.RouterGroup) {
	organizationGroup := router.Group("/organization")
	organizationGroup.Use(
		middleware.RequireActiveTenant(),
		middleware.RequireCustomerTenant(),
		permissionmiddleware.RequireOrganization(
			h.checker,
			"organization.feature.read",
		),
	)
	organizationGroup.GET("/features", h.ListFeatures)
	organizationGroup.GET("/usage", h.CheckUsage)

	platformGroup := router.Group("/platform/organizations")
	platformGroup.Use(middleware.RequirePlatformTenant())
	platformGroup.PATCH(
		"/:id/entitlements",
		permissionmiddleware.Require(
			h.platformPermChecker,
			"platform.organization.manage",
		),
		h.UpsertPlatformOverride,
	)
}

func (h *EntitlementHandler) ListFeatures(c *gin.Context) {
	var query dto.EntitlementListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	items, meta, err := h.service.ListFeatures(
		c.Request.Context(),
		tenantContext.OrganizationID(),
		query,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "organization features retrieved successfully", items, meta)
}

func (h *EntitlementHandler) CheckUsage(c *gin.Context) {
	var query dto.UsageQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	usage, err := h.service.CheckUsage(
		c.Request.Context(),
		tenantContext.OrganizationID(),
		query,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "organization usage retrieved successfully", usage)
}

func (h *EntitlementHandler) UpsertPlatformOverride(c *gin.Context) {
	var request dto.UpsertEntitlementRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	entitlement, err := h.service.UpsertPlatformOverride(
		c.Request.Context(),
		c.Param("id"),
		request,
		permissionmiddleware.UserID(c),
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "organization entitlement override updated successfully", entitlement)
}
