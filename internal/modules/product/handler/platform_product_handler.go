package handler

import (
	"context"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/product/dto"
	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

func validationHandlerError(err error) error {
	return coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity)
}

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

type PlatformProductHandler struct {
	plans        PlatformPlanService
	features     PlatformFeatureService
	entitlements PlatformPlanEntitlementService
	checker      permissionmiddleware.PermissionChecker
}

func NewPlatformProductHandler(
	plans PlatformPlanService,
	features PlatformFeatureService,
	entitlements PlatformPlanEntitlementService,
	checker permissionmiddleware.PermissionChecker,
) *PlatformProductHandler {
	return &PlatformProductHandler{
		plans:        plans,
		features:     features,
		entitlements: entitlements,
		checker:      checker,
	}
}

func (h *PlatformProductHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/platform/product")
	group.Use(middleware.RequirePlatformTenant())

	group.GET("/plans", permissionmiddleware.Require(h.checker, "platform.product.plan.read"), h.ListPlans)
	group.POST("/plans", permissionmiddleware.Require(h.checker, "platform.product.plan.manage"), h.CreatePlan)
	group.GET("/plans/:id", permissionmiddleware.Require(h.checker, "platform.product.plan.read"), h.GetPlan)
	group.PATCH("/plans/:id", permissionmiddleware.Require(h.checker, "platform.product.plan.manage"), h.UpdatePlan)
	group.DELETE("/plans/:id", permissionmiddleware.Require(h.checker, "platform.product.plan.manage"), h.DeletePlan)
	group.GET("/plans/:id/prices", permissionmiddleware.Require(h.checker, "platform.product.plan_price.read"), h.ListPlanPrices)
	group.POST("/plans/:id/prices", permissionmiddleware.Require(h.checker, "platform.product.plan_price.manage"), h.CreatePlanPrice)
	group.PATCH("/plans/:id/prices/:priceId", permissionmiddleware.Require(h.checker, "platform.product.plan_price.manage"), h.UpdatePlanPrice)
	group.DELETE("/plans/:id/prices/:priceId", permissionmiddleware.Require(h.checker, "platform.product.plan_price.manage"), h.DeletePlanPrice)
	group.GET("/plans/:id/entitlements", permissionmiddleware.Require(h.checker, "platform.product.entitlement.read"), h.ListPlanEntitlements)
	group.PUT("/plans/:id/entitlements", permissionmiddleware.Require(h.checker, "platform.product.entitlement.manage"), h.ReplacePlanEntitlements)

	group.GET("/features", permissionmiddleware.Require(h.checker, "platform.product.feature.read"), h.ListFeatures)
	group.POST("/features", permissionmiddleware.Require(h.checker, "platform.product.feature.manage"), h.CreateFeature)
	group.PATCH("/features/:id", permissionmiddleware.Require(h.checker, "platform.product.feature.manage"), h.UpdateFeature)
}

func (h *PlatformProductHandler) ListPlans(c *gin.Context) {
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
	response.JSON(c, http.StatusOK, "product plans retrieved successfully", result.Items, result.Meta)
}

func (h *PlatformProductHandler) CreatePlan(c *gin.Context) {
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
	corehttp.Created(c, "product plan created successfully", result)
}

func (h *PlatformProductHandler) GetPlan(c *gin.Context) {
	result, err := h.plans.FindByID(c.Request.Context(), c.Param("id"), true)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "product plan retrieved successfully", result)
}

func (h *PlatformProductHandler) UpdatePlan(c *gin.Context) {
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
	corehttp.OK(c, "product plan updated successfully", result)
}

func (h *PlatformProductHandler) DeletePlan(c *gin.Context) {
	if err := h.plans.Delete(c.Request.Context(), c.Param("id")); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "product plan deleted successfully", nil)
}

func (h *PlatformProductHandler) ListPlanPrices(c *gin.Context) {
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
	corehttp.OK(c, "product plan prices retrieved successfully", result)
}

func (h *PlatformProductHandler) CreatePlanPrice(c *gin.Context) {
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
	corehttp.Created(c, "product plan price created successfully", result)
}

func (h *PlatformProductHandler) UpdatePlanPrice(c *gin.Context) {
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
	corehttp.OK(c, "product plan price updated successfully", result)
}

func (h *PlatformProductHandler) DeletePlanPrice(c *gin.Context) {
	if err := h.plans.DeletePrice(c.Request.Context(), c.Param("id"), c.Param("priceId")); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "product plan price deleted successfully", nil)
}

func (h *PlatformProductHandler) ListPlanEntitlements(c *gin.Context) {
	result, err := h.entitlements.ListByPlanID(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "product plan entitlements retrieved successfully", result.Items)
}

func (h *PlatformProductHandler) ReplacePlanEntitlements(c *gin.Context) {
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
	corehttp.OK(c, "product plan entitlements updated successfully", result.Items)
}

func (h *PlatformProductHandler) ListFeatures(c *gin.Context) {
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
	response.JSON(c, http.StatusOK, "product features retrieved successfully", result.Items, result.Meta)
}

func (h *PlatformProductHandler) CreateFeature(c *gin.Context) {
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
	corehttp.Created(c, "product feature created successfully", result)
}

func (h *PlatformProductHandler) UpdateFeature(c *gin.Context) {
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
	corehttp.OK(c, "product feature updated successfully", result)
}
