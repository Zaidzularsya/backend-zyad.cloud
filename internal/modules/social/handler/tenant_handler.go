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

// TenantStaffLinks lets staff link CRM contacts to users and manage those links.
type TenantStaffLinks interface {
	Request(ctx context.Context, scope coretenant.Scope, actorUserID, contactID, userEmail string) (domain.CustomerLink, error)
	ListForContact(ctx context.Context, scope coretenant.Scope, contactID string) ([]domain.CustomerLink, error)
	Revoke(ctx context.Context, scope coretenant.Scope, actorUserID, linkID string) error
}

// Tenant write limits, per organization (spec §6.12). Claim invites send
// email, so they are the tightest.
const (
	inviteOrgLimit   = 30
	linkOrgLimit     = 60
	unlinkOrgLimit   = 120
	settingsOrgLimit = 30
	tenantRateWindow = time.Hour
)

// TenantHandler serves the tenant's own social settings and claim invites. The
// organization comes from the verified tenant scope, never from the request body.
type TenantHandler struct {
	settings TenantSettings
	invites  TenantInvites
	staff    TenantStaffLinks
	counter  middleware.RateCounter
}

func NewTenantHandler(settings TenantSettings, invites TenantInvites, staff TenantStaffLinks, counter middleware.RateCounter) *TenantHandler {
	return &TenantHandler{settings: settings, invites: invites, staff: staff, counter: counter}
}

// RegisterRoutes mounts on /app/social (tenant middleware applied by the router).
// Reading uses lead.read, the same permission that reads CRM settings.
func (h *TenantHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	router.GET("/settings", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.read"), h.GetSettings)
	router.PUT("/settings", permissionmiddleware.RequireOrganizationOrGlobal(p, "crm_settings.update"),
		h.perOrg("rl:social:settings:org:", settingsOrgLimit), h.UpdateSettings)
	if h.invites != nil {
		router.POST("/customer-invites", permissionmiddleware.RequireOrganizationOrGlobal(p, "contact.update"),
			h.perOrg("rl:social:invite:org:", inviteOrgLimit), h.SendCustomerInvite)
	}
	if h.staff != nil {
		update := permissionmiddleware.RequireOrganizationOrGlobal(p, "contact.update")
		router.POST("/customer-links", update, h.perOrg("rl:social:link:org:", linkOrgLimit), h.RequestCustomerLink)
		router.GET("/customer-links", update, h.ListCustomerLinks)
		router.DELETE("/customer-links/:id", update, h.perOrg("rl:social:unlink:org:", unlinkOrgLimit), h.RevokeCustomerLink)
	}
}

// perOrg rate limits a tenant write by the verified tenant organization.
func (h *TenantHandler) perOrg(prefix string, limit int) gin.HandlerFunc {
	return middleware.RateLimit(h.counter, prefix, limit, tenantRateWindow, func(c *gin.Context) string {
		if tc, ok := coretenant.FromContext(c.Request.Context()); ok {
			return tc.OrganizationID()
		}
		return "unknown"
	})
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

type requestCustomerLinkRequest struct {
	ContactID string `json:"contact_id" binding:"required,uuid"`
	UserEmail string `json:"user_email" binding:"required,email,max=254"`
}

// staffCustomerLinkResponse is a tenant-side view of one link. It carries the
// user id only: the user's other tenant relations are never exposed.
type staffCustomerLinkResponse struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	ContactID   string     `json:"contact_id"`
	Status      string     `json:"status"`
	VerifiedVia string     `json:"verified_via"`
	CreatedAt   time.Time  `json:"created_at"`
	ActivatedAt *time.Time `json:"activated_at"`
}

func toStaffCustomerLinkResponse(l domain.CustomerLink) staffCustomerLinkResponse {
	return staffCustomerLinkResponse{
		ID: l.ID, UserID: l.UserID, ContactID: l.ContactID, Status: l.Status,
		VerifiedVia: l.VerifiedVia, CreatedAt: l.CreatedAt, ActivatedAt: l.ActivatedAt,
	}
}

func (h *TenantHandler) RequestCustomerLink(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	var req requestCustomerLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	link, err := h.staff.Request(c.Request.Context(), scope, permissionmiddleware.UserID(c), req.ContactID, req.UserEmail)
	if err != nil {
		failSocial(c, "request customer link", err)
		return
	}
	corehttp.Created(c, "success", toStaffCustomerLinkResponse(link))
}

func (h *TenantHandler) ListCustomerLinks(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	contactID := c.Query("contact_id")
	if !isUUID(contactID) {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "contact_id must be a UUID", http.StatusUnprocessableEntity))
		return
	}
	links, err := h.staff.ListForContact(c.Request.Context(), scope, contactID)
	if err != nil {
		failSocial(c, "list customer links", err)
		return
	}
	out := make([]staffCustomerLinkResponse, 0, len(links))
	for _, l := range links {
		out = append(out, toStaffCustomerLinkResponse(l))
	}
	corehttp.OK(c, "success", out)
}

func (h *TenantHandler) RevokeCustomerLink(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	linkID := c.Param("id")
	if !isUUID(linkID) {
		failSocial(c, "revoke customer link", domain.ErrCustomerLinkNotFound)
		return
	}
	if err := h.staff.Revoke(c.Request.Context(), scope, permissionmiddleware.UserID(c), linkID); err != nil {
		failSocial(c, "revoke customer link", err)
		return
	}
	corehttp.NoContent(c)
}
