package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/dto"
	"zyad.cloud/internal/modules/landing/service"
)

type AdminDomainHandler struct {
	domainSvc service.DomainService
}

func NewAdminDomainHandler(domainSvc service.DomainService) *AdminDomainHandler {
	return &AdminDomainHandler{
		domainSvc: domainSvc,
	}
}

func (h *AdminDomainHandler) RegisterRoutes(router *gin.RouterGroup, checker permissionmiddleware.PermissionChecker) {
	router.GET("/admin/landing/domains/available", permissionmiddleware.Require(checker, "landing.domain.read"), h.ListAvailableDomains)
	
	router.GET("/admin/landing/domain-bindings", permissionmiddleware.Require(checker, "landing.domain.read"), h.ListDomainBindings)
	router.POST("/admin/landing/domain-bindings", permissionmiddleware.Require(checker, "landing.domain.manage"), h.CreateDomainBinding)
	router.PATCH("/admin/landing/domain-bindings/:id", permissionmiddleware.Require(checker, "landing.domain.manage"), h.UpdateDomainBinding)
	router.DELETE("/admin/landing/domain-bindings/:id", permissionmiddleware.Require(checker, "landing.domain.manage"), h.DeleteDomainBinding)
}

func (h *AdminDomainHandler) ListAvailableDomains(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	domains, err := h.domainSvc.ListAvailableDomains(c.Request.Context(), scope)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Available domains retrieved successfully", domains)
}

func (h *AdminDomainHandler) ListDomainBindings(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Query("landing_page_id")
	if pageID == "" {
		// MVP: Require pageID for now, or implement a global list.
		// Contract states `GET /admin/landing/domain-bindings`, which implies it could list globally.
		// But DomainService only has `ListBindings(ctx, scope, pageID string)`. 
		// For now, let's require landing_page_id.
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "landing_page_id query parameter is required", http.StatusUnprocessableEntity))
		return
	}

	bindings, err := h.domainSvc.ListBindings(c.Request.Context(), scope, pageID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Domain bindings retrieved successfully", bindings)
}

func (h *AdminDomainHandler) CreateDomainBinding(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	var req dto.DomainBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := service.BindDomainParams{
		OrganizationDomainID: req.OrganizationDomainID,
		LandingPageID:        req.LandingPageID,
		IsPrimary:            req.IsPrimary,
	}

	binding, err := h.domainSvc.BindDomain(c.Request.Context(), scope, params)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Domain bound successfully", binding)
}

func (h *AdminDomainHandler) UpdateDomainBinding(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	bindingID := c.Param("id")
	
	var req dto.DomainBindingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	// Currently, PATCH domain-binding only supports setting primary.
	if req.IsPrimary {
		// Note: DomainBindingRequest requires LandingPageID and OrganizationDomainID.
		// We use SetPrimaryBinding which needs pageID and bindingID.
		if req.LandingPageID == "" {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "landing_page_id is required to set primary binding", http.StatusUnprocessableEntity))
			return
		}
		
		err = h.domainSvc.SetPrimaryBinding(c.Request.Context(), scope, req.LandingPageID, bindingID)
		if err != nil {
			corehttp.Fail(c, err)
			return
		}
	}

	corehttp.OK(c, "Domain binding updated successfully", nil)
}

func (h *AdminDomainHandler) DeleteDomainBinding(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	bindingID := c.Param("id")

	if err := h.domainSvc.UnbindDomain(c.Request.Context(), scope, bindingID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Domain binding removed successfully", nil)
}
