package handler

import (
	"context"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/dto"
	"zyad.cloud/internal/modules/mikrotik/service"
)

// PolicyService is the subset of service.PolicyService used by the handler.
type PolicyService interface {
	SetExempt(ctx context.Context, scope coretenant.Scope, accountID string, exempt bool, reason, actor string) (domain.CustomerPolicy, error)
	GetPolicy(ctx context.Context, scope coretenant.Scope, accountID string) (domain.CustomerPolicy, error)
	PreviewTrigger(ctx context.Context, scope coretenant.Scope, productID string) (service.TriggerPreview, error)
	SetTrigger(ctx context.Context, scope coretenant.Scope, productID string, enabled bool, actor string) (domain.ProductTrigger, error)
	UnmarkedProducts(ctx context.Context, scope coretenant.Scope) ([]service.ProductInfo, error)
}

type PolicyHandler struct {
	svc PolicyService
}

func NewPolicyHandler(svc PolicyService) *PolicyHandler { return &PolicyHandler{svc: svc} }

// RegisterRoutes mounts the customer policy and product trigger routes. None
// of them contacts a router, so probeLimit is accepted only to keep the
// signature uniform with the other handlers.
func (h *PolicyHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker, _ gin.HandlerFunc) {
	router.GET("/customers/:account_id/policy", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_policy.read"), h.GetPolicy)
	router.PUT("/customers/:account_id/policy", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_policy.update"), h.SetPolicy)
	router.GET("/products/unmarked", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_product_trigger.read"), h.Unmarked)
	router.GET("/products/:id/trigger/preview", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_product_trigger.read"), h.PreviewTrigger)
	router.PUT("/products/:id/trigger", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_product_trigger.update"), h.SetTrigger)
}

func (h *PolicyHandler) GetPolicy(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	accountID := c.Param("account_id")
	if !validUUID(accountID) {
		failRouter(c, mikrotik.ErrAccountNotFound)
		return
	}
	p, err := h.svc.GetPolicy(c.Request.Context(), scope, accountID)
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.PolicyFromDomain(p))
}

func (h *PolicyHandler) SetPolicy(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var req dto.SetPolicyRequest
	if err := bindStrictJSON(c, &req); err != nil {
		validationFail(c, err)
		return
	}
	accountID := c.Param("account_id")
	if !validUUID(accountID) {
		failRouter(c, mikrotik.ErrAccountNotFound)
		return
	}
	p, err := h.svc.SetExempt(c.Request.Context(), scope, accountID, *req.Exempt, req.Reason, permissionmiddleware.UserID(c))
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.PolicyFromDomain(p))
}

func (h *PolicyHandler) Unmarked(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	products, err := h.svc.UnmarkedProducts(c.Request.Context(), scope)
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.ProductListFromService(products))
}

func (h *PolicyHandler) PreviewTrigger(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if !validUUID(id) {
		failRouter(c, mikrotik.ErrProductNotFound)
		return
	}
	preview, err := h.svc.PreviewTrigger(c.Request.Context(), scope, id)
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.TriggerPreviewFromService(preview))
}

func (h *PolicyHandler) SetTrigger(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var req dto.SetTriggerRequest
	if err := bindStrictJSON(c, &req); err != nil {
		validationFail(c, err)
		return
	}
	id := c.Param("id")
	if !validUUID(id) {
		failRouter(c, mikrotik.ErrProductNotFound)
		return
	}
	t, err := h.svc.SetTrigger(c.Request.Context(), scope, id, *req.Enabled, permissionmiddleware.UserID(c))
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.TriggerFromDomain(t))
}
