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

type OrganizationEntitlementService interface {
	ListFeatures(
		context.Context,
		string,
		dto.EntitlementListQuery,
	) ([]dto.EffectiveFeatureResponse, dto.PaginationMeta, error)
	CheckUsage(context.Context, string, dto.UsageQuery) (dto.UsageResponse, error)
	Summary(context.Context, string) ([]dto.EntitlementSummaryItem, error)
	UpsertPlatformOverride(
		context.Context,
		string,
		dto.UpsertEntitlementRequest,
		string,
	) (dto.EntitlementResponse, error)
}

type EntitlementHandler struct {
	service             OrganizationEntitlementService
	checker             permissionmiddleware.CombinedPermissionChecker
	platformPermChecker permissionmiddleware.PermissionChecker
}

func NewEntitlementHandler(
	service OrganizationEntitlementService,
	checker permissionmiddleware.CombinedPermissionChecker,
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

	// The summary is also served to the platform organization (everything
	// unlimited) because CRM/landing/billing pages are shared with it. Every
	// member may read it (spec 7b.3): no permission, but the caller must be an
	// authenticated, active member of the tenant organization.
	summaryGroup := router.Group("/organization")
	summaryGroup.Use(
		middleware.RequireActiveTenant(),
		middleware.RequireCustomerOrPlatformTenant(),
		requireTenantMember(),
	)
	summaryGroup.GET("/entitlements/summary", h.Summary)

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

// requireTenantMember admits only an authenticated user whose tenant context
// was resolved through an active membership (session/header/default
// membership). Host-resolved contexts carry no membership and are refused.
func requireTenantMember() gin.HandlerFunc {
	return func(c *gin.Context) {
		if permissionmiddleware.UserID(c) == "" {
			corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "user context is required", http.StatusUnauthorized))
			c.Abort()
			return
		}
		tenantContext, err := middleware.RequireTenantContext(c)
		if err != nil {
			corehttp.Fail(c, err)
			c.Abort()
			return
		}
		if tenantContext.MembershipID() == "" || tenantContext.MembershipStatus() != "active" {
			corehttp.Fail(c, coreerrors.New(
				"ORGANIZATION_ACCESS_DENIED",
				"user does not have an active membership in the organization",
				http.StatusForbidden,
			))
			c.Abort()
			return
		}
		c.Next()
	}
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

// Summary returns the per-feature entitlement and usage overview. The
// organization comes from the verified tenant context only.
func (h *EntitlementHandler) Summary(c *gin.Context) {
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	items, err := h.service.Summary(c.Request.Context(), tenantContext.OrganizationID())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "organization entitlement summary retrieved successfully", items)
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
