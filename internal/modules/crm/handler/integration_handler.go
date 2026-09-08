package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/modules/crm/dto"
	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/modules/crm/service"
	"zyad.cloud/internal/shared/response"
)

type IntegrationHandler struct {
	svc service.IntegrationService
}

func NewIntegrationHandler(svc service.IntegrationService) *IntegrationHandler {
	return &IntegrationHandler{svc: svc}
}

// RegisterRoutes registers integration routes under the given parent group.
// See CompanyHandler.RegisterRoutes for the tenant/entitlement guard note.
//
// GET/PUT .../secret are guarded by separate permissions
// (integration.view_secret / integration.update_secret) from the rest of the
// resource — see docs/reference-crm.md "Security Baseline".
func (h *IntegrationHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/integrations")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "integration.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "integration.create"), h.Create)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "integration.read"), h.Get)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "integration.update"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "integration.delete"), h.Delete)
	group.POST("/:id/connect", permissionmiddleware.RequireOrganizationOrGlobal(p, "integration.connect"), h.Connect)
	group.GET("/:id/secret", permissionmiddleware.RequireOrganizationOrGlobal(p, "integration.view_secret"), h.RevealSecret)
	group.PUT("/:id/secret", permissionmiddleware.RequireOrganizationOrGlobal(p, "integration.update_secret"), h.UpdateSecret)
}

func (h *IntegrationHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var query dto.IntegrationListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	page := query.Page
	if page <= 0 {
		page = 1
	}
	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 20
	}

	integrations, total, err := h.svc.List(c.Request.Context(), scope, repository.IntegrationListFilter{
		Provider: domain.IntegrationProvider(query.Provider),
		Limit:    perPage,
		Offset:   (page - 1) * perPage,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusOK, "success", dto.IntegrationListFromDomain(integrations), dto.BuildMeta(page, perPage, total))
}

func (h *IntegrationHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.CreateIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	integ, err := h.svc.Create(c.Request.Context(), scope, service.CreateIntegrationInput{
		Provider:  domain.IntegrationProvider(req.Provider),
		Name:      req.Name,
		Config:    req.Config,
		Secret:    req.Secret,
		CreatedBy: userID,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidIntegrationProvider) {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
			return
		}
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.IntegrationFromDomain(integ))
}

func (h *IntegrationHandler) Get(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	integ, err := h.svc.Get(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.IntegrationFromDomain(integ))
}

func (h *IntegrationHandler) Update(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.UpdateIntegrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	integ, err := h.svc.Update(c.Request.Context(), scope, c.Param("id"), service.UpdateIntegrationInput{
		Name:      req.Name,
		Config:    req.Config,
		IsActive:  req.IsActive,
		UpdatedBy: userID,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.IntegrationFromDomain(integ))
}

func (h *IntegrationHandler) Delete(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	if err := h.svc.Delete(c.Request.Context(), scope, c.Param("id"), userID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "deleted", nil)
}

func (h *IntegrationHandler) Connect(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	integ, err := h.svc.Connect(c.Request.Context(), scope, c.Param("id"), userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.IntegrationFromDomain(integ))
}

func (h *IntegrationHandler) RevealSecret(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	secret, err := h.svc.RevealSecret(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.IntegrationSecretResponse{Secret: secret})
}

func (h *IntegrationHandler) UpdateSecret(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.UpdateIntegrationSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	integ, err := h.svc.UpdateSecret(c.Request.Context(), scope, c.Param("id"), req.Secret, userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.IntegrationFromDomain(integ))
}
