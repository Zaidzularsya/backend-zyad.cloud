package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/billing/dto"
	"zyad.cloud/internal/modules/billing/model"
	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type PlatformPlanService interface {
	List(ctx context.Context, query dto.PlanListQuery) (dto.PlanListResponse, error)
	FindByID(ctx context.Context, id string, includePrices bool) (dto.PlanResponse, error)
	Create(ctx context.Context, request dto.CreatePlanRequest) (dto.PlanResponse, error)
	Update(ctx context.Context, id string, request dto.UpdatePlanRequest) (dto.PlanResponse, error)
	Delete(ctx context.Context, id string) error
	ListPrices(ctx context.Context, planID string, includeDeleted bool) ([]dto.PlanPriceResponse, error)
	CreatePrice(ctx context.Context, planID string, request dto.CreatePlanPriceRequest) (dto.PlanPriceResponse, error)
	UpdatePrice(ctx context.Context, planID string, priceID string, request dto.UpdatePlanPriceRequest) (dto.PlanPriceResponse, error)
	DeletePrice(ctx context.Context, planID string, priceID string) error
}

type PlatformFeatureService interface {
	List(ctx context.Context, query dto.FeatureListQuery) (dto.FeatureListResponse, error)
	Create(ctx context.Context, request dto.CreateFeatureRequest) (dto.FeatureResponse, error)
	Update(ctx context.Context, id string, request dto.UpdateFeatureRequest) (dto.FeatureResponse, error)
}

type PlatformPlanEntitlementService interface {
	ListByPlanID(ctx context.Context, planID string) (dto.PlanEntitlementListResponse, error)
	ReplaceByPlanID(
		ctx context.Context,
		planID string,
		request dto.ReplacePlanEntitlementsRequest,
	) (dto.PlanEntitlementListResponse, error)
}

type PlatformSubscriptionService interface {
	List(ctx context.Context, query dto.SubscriptionListQuery) (dto.SubscriptionListResponse, error)
	Create(ctx context.Context, request dto.CreateSubscriptionRequest) (dto.SubscriptionResponse, error)
	UpdateByID(ctx context.Context, id string, request dto.UpdateSubscriptionRequest) (dto.SubscriptionResponse, error)
	ChangeStatusByID(
		ctx context.Context,
		id string,
		status model.SubscriptionStatus,
		actorUserID string,
		reason string,
	) (dto.SubscriptionResponse, error)
}

type PlatformInvoiceService interface {
	List(ctx context.Context, query dto.InvoiceListQuery) (dto.InvoiceListResponse, error)
	Create(ctx context.Context, request dto.CreateInvoiceRequest) (dto.InvoiceResponse, error)
}

type PlatformPaymentService interface {
	MarkInvoicePaidByID(
		ctx context.Context,
		invoiceID string,
		request dto.MarkInvoicePaidRequest,
	) (dto.PaymentResponse, error)
}

type PlatformBillingHandler struct {
	plans         PlatformPlanService
	features      PlatformFeatureService
	entitlements  PlatformPlanEntitlementService
	subscriptions PlatformSubscriptionService
	invoices      PlatformInvoiceService
	payments      PlatformPaymentService
	checker       permissionmiddleware.PermissionChecker
}

func NewPlatformBillingHandler(
	plans PlatformPlanService,
	features PlatformFeatureService,
	entitlements PlatformPlanEntitlementService,
	subscriptions PlatformSubscriptionService,
	invoices PlatformInvoiceService,
	payments PlatformPaymentService,
	checker permissionmiddleware.PermissionChecker,
) *PlatformBillingHandler {
	return &PlatformBillingHandler{
		plans:         plans,
		features:      features,
		entitlements:  entitlements,
		subscriptions: subscriptions,
		invoices:      invoices,
		payments:      payments,
		checker:       checker,
	}
}

func (h *PlatformBillingHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/platform/billing")
	group.Use(middleware.RequirePlatformTenant())

	group.GET("/plans", permissionmiddleware.Require(h.checker, "platform.billing.plan.read"), h.ListPlans)
	group.POST("/plans", permissionmiddleware.Require(h.checker, "platform.billing.plan.manage"), h.CreatePlan)
	group.GET("/plans/:id", permissionmiddleware.Require(h.checker, "platform.billing.plan.read"), h.GetPlan)
	group.PATCH("/plans/:id", permissionmiddleware.Require(h.checker, "platform.billing.plan.manage"), h.UpdatePlan)
	group.DELETE("/plans/:id", permissionmiddleware.Require(h.checker, "platform.billing.plan.manage"), h.DeletePlan)
	group.GET("/plans/:id/prices", permissionmiddleware.Require(h.checker, "platform.billing.plan_price.read"), h.ListPlanPrices)
	group.POST("/plans/:id/prices", permissionmiddleware.Require(h.checker, "platform.billing.plan_price.manage"), h.CreatePlanPrice)
	group.PATCH("/plans/:id/prices/:priceId", permissionmiddleware.Require(h.checker, "platform.billing.plan_price.manage"), h.UpdatePlanPrice)
	group.DELETE("/plans/:id/prices/:priceId", permissionmiddleware.Require(h.checker, "platform.billing.plan_price.manage"), h.DeletePlanPrice)
	group.GET("/plans/:id/entitlements", permissionmiddleware.Require(h.checker, "platform.billing.entitlement.read"), h.ListPlanEntitlements)
	group.PUT("/plans/:id/entitlements", permissionmiddleware.Require(h.checker, "platform.billing.entitlement.manage"), h.ReplacePlanEntitlements)

	group.GET("/features", permissionmiddleware.Require(h.checker, "platform.billing.feature.read"), h.ListFeatures)
	group.POST("/features", permissionmiddleware.Require(h.checker, "platform.billing.feature.manage"), h.CreateFeature)
	group.PATCH("/features/:id", permissionmiddleware.Require(h.checker, "platform.billing.feature.manage"), h.UpdateFeature)

	group.GET("/subscriptions", permissionmiddleware.Require(h.checker, "platform.billing.subscription.read"), h.ListSubscriptions)
	group.POST("/subscriptions", permissionmiddleware.Require(h.checker, "platform.billing.subscription.manage"), h.CreateSubscription)
	group.PATCH("/subscriptions/:id", permissionmiddleware.Require(h.checker, "platform.billing.subscription.manage"), h.UpdateSubscription)
	group.POST("/subscriptions/:id/cancel", permissionmiddleware.Require(h.checker, "platform.billing.subscription.manage"), h.CancelSubscription)
	group.POST("/subscriptions/:id/suspend", permissionmiddleware.Require(h.checker, "platform.billing.subscription.manage"), h.SuspendSubscription)

	group.GET("/invoices", permissionmiddleware.Require(h.checker, "platform.billing.invoice.read"), h.ListInvoices)
	group.POST("/invoices", permissionmiddleware.Require(h.checker, "platform.billing.invoice.manage"), h.CreateInvoice)
	group.POST("/invoices/:id/mark-paid", permissionmiddleware.Require(h.checker, "platform.billing.payment.manage"), h.MarkInvoicePaid)
}

func (h *PlatformBillingHandler) ListPlans(c *gin.Context) {
	var query dto.PlanListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.plans.List(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "billing plans retrieved successfully", result.Items, result.Meta)
}

func (h *PlatformBillingHandler) CreatePlan(c *gin.Context) {
	var request dto.CreatePlanRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.plans.Create(c.Request.Context(), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "billing plan created successfully", result)
}

func (h *PlatformBillingHandler) GetPlan(c *gin.Context) {
	result, err := h.plans.FindByID(c.Request.Context(), c.Param("id"), true)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing plan retrieved successfully", result)
}

func (h *PlatformBillingHandler) UpdatePlan(c *gin.Context) {
	var request dto.UpdatePlanRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.plans.Update(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing plan updated successfully", result)
}

func (h *PlatformBillingHandler) DeletePlan(c *gin.Context) {
	if err := h.plans.Delete(c.Request.Context(), c.Param("id")); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing plan deleted successfully", nil)
}

func (h *PlatformBillingHandler) ListPlanPrices(c *gin.Context) {
	var query dto.PlanPriceListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.plans.ListPrices(c.Request.Context(), c.Param("id"), query.IncludeDeleted)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing plan prices retrieved successfully", result)
}

func (h *PlatformBillingHandler) CreatePlanPrice(c *gin.Context) {
	var request dto.CreatePlanPriceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.plans.CreatePrice(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "billing plan price created successfully", result)
}

func (h *PlatformBillingHandler) UpdatePlanPrice(c *gin.Context) {
	var request dto.UpdatePlanPriceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.plans.UpdatePrice(c.Request.Context(), c.Param("id"), c.Param("priceId"), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing plan price updated successfully", result)
}

func (h *PlatformBillingHandler) DeletePlanPrice(c *gin.Context) {
	if err := h.plans.DeletePrice(c.Request.Context(), c.Param("id"), c.Param("priceId")); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing plan price deleted successfully", nil)
}

func (h *PlatformBillingHandler) ListPlanEntitlements(c *gin.Context) {
	result, err := h.entitlements.ListByPlanID(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing plan entitlements retrieved successfully", result.Items)
}

func (h *PlatformBillingHandler) ReplacePlanEntitlements(c *gin.Context) {
	var request dto.ReplacePlanEntitlementsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.entitlements.ReplaceByPlanID(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing plan entitlements updated successfully", result.Items)
}

func (h *PlatformBillingHandler) ListFeatures(c *gin.Context) {
	var query dto.FeatureListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.features.List(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "billing features retrieved successfully", result.Items, result.Meta)
}

func (h *PlatformBillingHandler) CreateFeature(c *gin.Context) {
	var request dto.CreateFeatureRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.features.Create(c.Request.Context(), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "billing feature created successfully", result)
}

func (h *PlatformBillingHandler) UpdateFeature(c *gin.Context) {
	var request dto.UpdateFeatureRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.features.Update(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing feature updated successfully", result)
}

func (h *PlatformBillingHandler) ListSubscriptions(c *gin.Context) {
	var query dto.SubscriptionListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.subscriptions.List(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "billing subscriptions retrieved successfully", result.Items, result.Meta)
}

func (h *PlatformBillingHandler) CreateSubscription(c *gin.Context) {
	var request dto.CreateSubscriptionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.subscriptions.Create(c.Request.Context(), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "billing subscription created successfully", result)
}

func (h *PlatformBillingHandler) UpdateSubscription(c *gin.Context) {
	var request dto.UpdateSubscriptionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.subscriptions.UpdateByID(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing subscription updated successfully", result)
}

func (h *PlatformBillingHandler) CancelSubscription(c *gin.Context) {
	reason := struct {
		Reason string `json:"reason"`
	}{}
	if err := c.ShouldBindJSON(&reason); err != nil && !errors.Is(err, io.EOF) {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.subscriptions.ChangeStatusByID(
		c.Request.Context(),
		c.Param("id"),
		model.SubscriptionStatusCanceled,
		permissionmiddleware.UserID(c),
		reason.Reason,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing subscription canceled successfully", result)
}

func (h *PlatformBillingHandler) SuspendSubscription(c *gin.Context) {
	reason := struct {
		Reason string `json:"reason"`
	}{}
	if err := c.ShouldBindJSON(&reason); err != nil && !errors.Is(err, io.EOF) {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.subscriptions.ChangeStatusByID(
		c.Request.Context(),
		c.Param("id"),
		model.SubscriptionStatusSuspended,
		permissionmiddleware.UserID(c),
		reason.Reason,
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing subscription suspended successfully", result)
}

func (h *PlatformBillingHandler) ListInvoices(c *gin.Context) {
	var query dto.InvoiceListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.invoices.List(c.Request.Context(), query)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "billing invoices retrieved successfully", result.Items, result.Meta)
}

func (h *PlatformBillingHandler) CreateInvoice(c *gin.Context) {
	var request dto.CreateInvoiceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.invoices.Create(c.Request.Context(), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "billing invoice created successfully", result)
}

func (h *PlatformBillingHandler) MarkInvoicePaid(c *gin.Context) {
	var request dto.MarkInvoicePaidRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	result, err := h.payments.MarkInvoicePaidByID(c.Request.Context(), c.Param("id"), request)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "billing invoice marked as paid successfully", result)
}
