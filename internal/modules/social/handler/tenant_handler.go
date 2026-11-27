package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/social/domain"
)

type TenantSettings interface {
	Get(ctx context.Context, organizationID string) (domain.TenantSettings, error)
	SetAutoOffer(ctx context.Context, organizationID string, autoOffer bool, actorUserID string) (domain.TenantSettings, error)
}

// TenantHandler serves the tenant's own social settings. The organization comes
// from the verified tenant scope, never from the request body.
type TenantHandler struct {
	settings TenantSettings
}

func NewTenantHandler(settings TenantSettings) *TenantHandler {
	return &TenantHandler{settings: settings}
}

// RegisterRoutes mounts on /app/social (tenant middleware applied by the router).
// Reading uses lead.read, the same permission that reads CRM settings.
func (h *TenantHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	router.GET("/settings", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.read"), h.GetSettings)
	router.PUT("/settings", permissionmiddleware.RequireOrganizationOrGlobal(p, "crm_settings.update"), h.UpdateSettings)
}

type tenantSettingsResponse struct {
	CustomerClaimAutoOffer bool       `json:"customer_claim_auto_offer"`
	UpdatedAt              *time.Time `json:"updated_at"`
}

type updateTenantSettingsRequest struct {
	CustomerClaimAutoOffer *bool `json:"customer_claim_auto_offer" binding:"required"`
}

func (h *TenantHandler) GetSettings(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	s, err := h.settings.Get(c.Request.Context(), scope.OrganizationID())
	if err != nil {
		failSocial(c, "get social settings", err)
		return
	}
	corehttp.OK(c, "success", tenantSettingsResponse{CustomerClaimAutoOffer: s.CustomerClaimAutoOffer, UpdatedAt: s.UpdatedAt})
}

func (h *TenantHandler) UpdateSettings(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	var req updateTenantSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	s, err := h.settings.SetAutoOffer(c.Request.Context(), scope.OrganizationID(), *req.CustomerClaimAutoOffer, permissionmiddleware.UserID(c))
	if err != nil {
		failSocial(c, "update social settings", err)
		return
	}
	corehttp.OK(c, "success", tenantSettingsResponse{CustomerClaimAutoOffer: s.CustomerClaimAutoOffer, UpdatedAt: s.UpdatedAt})
}
