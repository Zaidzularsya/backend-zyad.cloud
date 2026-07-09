package handler

import (
	"context"
	"net/http"

	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/billing/dto"
	subscriptiondto "zyad.cloud/internal/modules/subscription/dto"
	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type TenantBillingService interface {
	CurrentPlan(ctx context.Context, organizationID string) (dto.CurrentPlanResponse, error)
	CheckUsage(
		ctx context.Context,
		organizationID string,
		query dto.UsageQuery,
	) (dto.UsageItemResponse, error)
	ListInvoices(
		ctx context.Context,
		organizationID string,
		query dto.InvoiceListQuery,
	) (dto.InvoiceListResponse, error)
	RequestUpgrade(
		ctx context.Context,
		organizationID string,
		actorUserID string,
		request dto.UpgradeSubscriptionRequest,
	) (dto.InvoiceResponse, error)
	CancelCurrentSubscription(
		ctx context.Context,
		organizationID string,
		actorUserID string,
		reason string,
	) (subscriptiondto.SubscriptionResponse, error)
}

type TenantBillingHandler struct {
	service TenantBillingService
	checker permissionmiddleware.OrganizationPermissionChecker
}

func NewTenantBillingHandler(
	service TenantBillingService,
	checker permissionmiddleware.OrganizationPermissionChecker,
) *TenantBillingHandler {
	return &TenantBillingHandler{service: service, checker: checker}
}

func (h *TenantBillingHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/app/billing")
	group.Use(
		middleware.RequireActiveTenant(),
		middleware.RequireCustomerTenant(),
	)
	group.GET(
		"/current-plan",
		permissionmiddleware.RequireOrganization(h.checker, "organization.billing.read"),
		h.CurrentPlan,
	)
	group.GET(
		"/invoices",
		permissionmiddleware.RequireOrganization(h.checker, "organization.billing.read"),
		h.ListInvoices,
	)
	group.GET(
		"/usage",
		permissionmiddleware.RequireOrganization(h.checker, "organization.billing.read"),
		h.CheckUsage,
	)
	group.POST(
		"/upgrade",
		permissionmiddleware.RequireOrganization(h.checker, "organization.billing.manage"),
		h.RequestUpgrade,
	)
	group.POST(
		"/cancel",
		permissionmiddleware.RequireOrganization(h.checker, "organization.billing.manage"),
		h.CancelSubscription,
	)
}

func (h *TenantBillingHandler) CurrentPlan(c *gin.Context) {
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	result, err := h.service.CurrentPlan(c.Request.Context(), tenantContext.OrganizationID())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "current billing plan retrieved successfully", result)
}

func (h *TenantBillingHandler) ListInvoices(c *gin.Context) {
	var query dto.InvoiceListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	result, err := h.service.ListInvoices(c.Request.Context(), tenantContext.OrganizationID(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "billing invoices retrieved successfully", result.Items, result.Meta)
}

func (h *TenantBillingHandler) CheckUsage(c *gin.Context) {
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
	result, err := h.service.CheckUsage(c.Request.Context(), tenantContext.OrganizationID(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing usage retrieved successfully", result)
}

func (h *TenantBillingHandler) RequestUpgrade(c *gin.Context) {
	var request dto.UpgradeSubscriptionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	result, err := h.service.RequestUpgrade(
		c.Request.Context(),
		tenantContext.OrganizationID(),
		permissionmiddleware.UserID(c),
		request,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "billing upgrade invoice created successfully", result)
}

func (h *TenantBillingHandler) CancelSubscription(c *gin.Context) {
	var request dto.CancelSubscriptionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	tenantContext, err := middleware.RequireTenantContext(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	result, err := h.service.CancelCurrentSubscription(
		c.Request.Context(),
		tenantContext.OrganizationID(),
		permissionmiddleware.UserID(c),
		request.Reason,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing subscription scheduled for cancellation successfully", result)
}
