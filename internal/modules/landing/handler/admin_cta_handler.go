package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/dto"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
)

type AdminCTAHandler struct {
	ctaService service.CTAService
}

func NewAdminCTAHandler(ctaService service.CTAService) *AdminCTAHandler {
	return &AdminCTAHandler{
		ctaService: ctaService,
	}
}

func (h *AdminCTAHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.PermissionChecker) {
	group := router.Group("/admin/landing/ctas")

	group.GET("", permissionmiddleware.Require(p, "landing.cta.manage"), h.List)
	group.POST("", permissionmiddleware.Require(p, "landing.cta.manage"), h.Create)
	group.PATCH("/:id", permissionmiddleware.Require(p, "landing.cta.manage"), h.Update)
	group.DELETE("/:id", permissionmiddleware.Require(p, "landing.cta.manage"), h.Delete)
}

func (h *AdminCTAHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	ctas, err := h.ctaService.List(c.Request.Context(), scope)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", ctas)
}

func (h *AdminCTAHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)

	var req dto.CTARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := repository.CreateCTAParams{
		Name:        req.Name,
		Label:       req.Label,
		Type:        domain.CTAType(req.Type),
		Target:      domain.CTATarget(req.Target),
		Destination: req.Destination,
		TrackingKey: req.TrackingKey,
		CreatedBy:   userID,
	}

	cta, err := h.ctaService.Create(c.Request.Context(), scope, params)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", cta)
}

func (h *AdminCTAHandler) Update(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)

	id := c.Param("id")

	// Create a dynamic update request type or just use map and selectively map
	// For MVP, we will use a map structure or create a specific UpdateCTARequest
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := repository.UpdateCTAParams{
		UpdatedBy: userID,
	}

	if v, ok := req["name"].(string); ok {
		params.Name = &v
	}
	if v, ok := req["label"].(string); ok {
		params.Label = &v
	}
	if v, ok := req["type"].(string); ok {
		t := domain.CTAType(v)
		params.Type = &t
	}
	if v, ok := req["target"].(string); ok {
		t := domain.CTATarget(v)
		params.Target = &t
	}
	if v, ok := req["destination"].(string); ok {
		params.Destination = &v
	}
	if v, ok := req["tracking_key"].(string); ok {
		params.TrackingKey = &v
	}

	cta, err := h.ctaService.Update(c.Request.Context(), scope, id, params)
	if err != nil {
		if err == service.ErrInvalidCTAType || err == service.ErrInvalidTarget {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusBadRequest))
			return
		}
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", cta)
}

func (h *AdminCTAHandler) Delete(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)

	id := c.Param("id")

	if err := h.ctaService.Delete(c.Request.Context(), scope, id, userID); err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "deleted", nil)
}
