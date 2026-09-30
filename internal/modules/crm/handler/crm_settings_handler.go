package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/dto"
	"zyad.cloud/internal/modules/crm/service"
)

type CRMSettingsHandler struct{ svc service.CRMSettingsService }

func NewCRMSettingsHandler(svc service.CRMSettingsService) *CRMSettingsHandler {
	return &CRMSettingsHandler{svc: svc}
}

func (h *CRMSettingsHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	router.GET("/settings", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.read"), h.Get)
	router.PATCH("/settings", permissionmiddleware.RequireOrganizationOrGlobal(p, "crm_settings.update"), h.Update)
}

func (h *CRMSettingsHandler) Get(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	s, err := h.svc.Get(c.Request.Context(), scope)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.CRMSettingsResponse{LeadPlaybookEnabled: s.LeadPlaybookEnabled, UpdatedAt: s.UpdatedAt})
}

func (h *CRMSettingsHandler) Update(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	var req dto.UpdateCRMSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	s, err := h.svc.Update(c.Request.Context(), scope, *req.LeadPlaybookEnabled, permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.CRMSettingsResponse{LeadPlaybookEnabled: s.LeadPlaybookEnabled, UpdatedAt: s.UpdatedAt})
}
