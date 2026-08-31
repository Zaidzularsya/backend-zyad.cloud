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

type AdminIntegrationHandler struct {
	deliveryService service.DeliveryService
}

func NewAdminIntegrationHandler(deliveryService service.DeliveryService) *AdminIntegrationHandler {
	return &AdminIntegrationHandler{
		deliveryService: deliveryService,
	}
}

func (h *AdminIntegrationHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/admin/landing")

	// Integrations
	group.GET("/lead-integrations", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.integration.read"), h.ListIntegrations)
	group.POST("/lead-integrations", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.integration.manage"), h.CreateIntegration)
	group.PATCH("/lead-integrations/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.integration.manage"), h.UpdateIntegration)
	group.DELETE("/lead-integrations/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.integration.manage"), h.DeleteIntegration)
	group.POST("/lead-integrations/:id/test", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.integration.manage"), h.TestIntegration)

	// Deliveries
	group.GET("/lead-deliveries", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.integration.read"), h.ListDeliveries)
	group.POST("/lead-deliveries/:id/retry", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.integration.manage"), h.RetryDelivery)
}

func (h *AdminIntegrationHandler) ListIntegrations(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	integrations, err := h.deliveryService.ListIntegrations(c.Request.Context(), scope)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", integrations)
}

func (h *AdminIntegrationHandler) CreateIntegration(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)

	var req dto.LeadIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := repository.CreateIntegrationParams{
		Name:         req.Name,
		Type:         domain.IntegrationType(req.Type),
		Credentials:  req.Credentials,
		EventFilters: req.EventFilters,
		IsActive:     req.IsActive,
		CreatedBy:    userID,
	}

	integration, err := h.deliveryService.CreateIntegration(c.Request.Context(), scope, params)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", integration)
}

func (h *AdminIntegrationHandler) UpdateIntegration(c *gin.Context) {
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

	params := repository.UpdateIntegrationParams{
		UpdatedBy: userID,
	}

	if v, ok := req["name"].(string); ok {
		params.Name = v
	}
	if v, ok := req["credentials"].(map[string]any); ok {
		params.Credentials = v
	}
	if v, ok := req["event_filters"].([]any); ok {
		params.EventFilters = v
	}
	if v, ok := req["is_active"].(bool); ok {
		params.IsActive = v
	}

	err = h.deliveryService.UpdateIntegration(c.Request.Context(), scope, id, params)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	// Fetch to return updated response
	integration, err := h.deliveryService.FindIntegrationByID(c.Request.Context(), scope, id)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", integration)
}

func (h *AdminIntegrationHandler) DeleteIntegration(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	id := c.Param("id")

	if err := h.deliveryService.DeleteIntegration(c.Request.Context(), scope, id); err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "deleted", nil)
}

func (h *AdminIntegrationHandler) TestIntegration(c *gin.Context) {
	corehttp.Fail(c, coreerrors.New("NOT_IMPLEMENTED", "test integration is not implemented", http.StatusNotImplemented))
}

func (h *AdminIntegrationHandler) ListDeliveries(c *gin.Context) {
	corehttp.Fail(c, coreerrors.New("NOT_IMPLEMENTED", "list deliveries is not implemented", http.StatusNotImplemented))
}

func (h *AdminIntegrationHandler) RetryDelivery(c *gin.Context) {
	corehttp.Fail(c, coreerrors.New("NOT_IMPLEMENTED", "retry delivery is not implemented", http.StatusNotImplemented))
}
