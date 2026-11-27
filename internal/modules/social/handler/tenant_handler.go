package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/social/domain"
)

type TenantSettings interface {
	Get(ctx context.Context, organizationID string) (domain.TenantSettings, error)
	SetAutoOffer(ctx context.Context, organizationID string, autoOffer bool, actorUserID string) (domain.TenantSettings, error)
}

// TenantInvites sends claim invites to the tenant's CRM contacts.
type TenantInvites interface {
	Send(ctx context.Context, scope coretenant.Scope, actorUserID, contactID string) error
}

// Claim invites send email, so they are capped per organization.
const (
	inviteOrgLimit  = 30
	inviteOrgWindow = time.Hour
)

// TenantHandler serves the tenant's own social settings and claim invites. The
// organization comes from the verified tenant scope, never from the request body.
type TenantHandler struct {
	settings TenantSettings
	invites  TenantInvites
	counter  middleware.RateCounter
}

func NewTenantHandler(settings TenantSettings, invites TenantInvites, counter middleware.RateCounter) *TenantHandler {
	return &TenantHandler{settings: settings, invites: invites, counter: counter}
}

// RegisterRoutes mounts on /app/social (tenant middleware applied by the router).
// Reading uses lead.read, the same permission that reads CRM settings.
func (h *TenantHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	router.GET("/settings", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.read"), h.GetSettings)
	router.PUT("/settings", permissionmiddleware.RequireOrganizationOrGlobal(p, "crm_settings.update"), h.UpdateSettings)
	if h.invites != nil {
		perOrg := middleware.RateLimit(h.counter, "rl:social:invite:org:", inviteOrgLimit, inviteOrgWindow,
			func(c *gin.Context) string {
				if tc, ok := coretenant.FromContext(c.Request.Context()); ok {
					return tc.OrganizationID()
				}
				return "unknown"
			})
		router.POST("/customer-invites", permissionmiddleware.RequireOrganizationOrGlobal(p, "contact.update"), perOrg, h.SendCustomerInvite)
	}
}

type sendCustomerInviteRequest struct {
	ContactID string `json:"contact_id" binding:"required,uuid"`
}

func (h *TenantHandler) SendCustomerInvite(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	var req sendCustomerInviteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	if err := h.invites.Send(c.Request.Context(), scope, permissionmiddleware.UserID(c), req.ContactID); err != nil {
		failSocial(c, "send customer claim invite", err)
		return
	}
	corehttp.NoContent(c)
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
