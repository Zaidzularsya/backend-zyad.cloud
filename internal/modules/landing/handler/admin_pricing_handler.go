package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/dto"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
)

// AdminPricingHandler manages a tenant's own pricing-plan cards, shown by the
// `zyad-pricing-plans` GrapesJS block. Reuses the "landing.menu.manage"
// permission — same audience as tenant nav/footer content management.
type AdminPricingHandler struct {
	pricingService service.PricingService
}

func NewAdminPricingHandler(pricingService service.PricingService) *AdminPricingHandler {
	return &AdminPricingHandler{
		pricingService: pricingService,
	}
}

func (h *AdminPricingHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/admin/landing/pricing-plans")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.menu.manage"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.menu.manage"), h.Create)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.menu.manage"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.menu.manage"), h.Delete)
	group.PUT("/reorder", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.menu.manage"), h.Reorder)
}

func (h *AdminPricingHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	plans, err := h.pricingService.List(c.Request.Context(), scope)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", plans)
}

func (h *AdminPricingHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)

	var req dto.PricingPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := repository.CreatePricingPlanParams{
		Name:          req.Name,
		PriceLabel:    req.PriceLabel,
		IntervalLabel: req.IntervalLabel,
		Description:   req.Description,
		Features:      req.Features,
		CTALabel:      req.CTALabel,
		CTAURL:        req.CTAURL,
		IsFeatured:    req.IsFeatured,
		IsEnabled:     req.IsEnabled,
		CreatedBy:     userID,
	}

	plan, err := h.pricingService.Create(c.Request.Context(), scope, params)
	if err != nil {
		if err == service.ErrPricingPlanNameRequired || err == service.ErrPricingPlanPriceRequired {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusBadRequest))
			return
		}
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", plan)
}

func (h *AdminPricingHandler) Update(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)
	id := c.Param("id")

	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := repository.UpdatePricingPlanParams{UpdatedBy: userID}

	if v, ok := req["name"].(string); ok {
		params.Name = &v
	}
	if v, ok := req["price_label"].(string); ok {
		params.PriceLabel = &v
	}
	if v, ok := req["interval_label"].(string); ok {
		params.IntervalLabel = &v
	}
	if v, ok := req["description"].(string); ok {
		params.Description = &v
	}
	if raw, ok := req["features"].([]any); ok {
		features := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok {
				features = append(features, s)
			}
		}
		params.Features = &features
	}
	if v, ok := req["cta_label"].(string); ok {
		params.CTALabel = &v
	}
	if v, ok := req["cta_url"].(string); ok {
		params.CTAURL = &v
	}
	if v, ok := req["is_featured"].(bool); ok {
		params.IsFeatured = &v
	}
	if v, ok := req["is_enabled"].(bool); ok {
		params.IsEnabled = &v
	}

	plan, err := h.pricingService.Update(c.Request.Context(), scope, id, params)
	if err != nil {
		if err == service.ErrPricingPlanNameRequired || err == service.ErrPricingPlanPriceRequired {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusBadRequest))
			return
		}
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", plan)
}

func (h *AdminPricingHandler) Delete(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)
	id := c.Param("id")

	if err := h.pricingService.Delete(c.Request.Context(), scope, id, userID); err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "deleted", nil)
}

func (h *AdminPricingHandler) Reorder(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var req dto.PricingPlanReorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	if err := h.pricingService.Reorder(c.Request.Context(), scope, req.PlanIDs); err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "reordered", nil)
}
