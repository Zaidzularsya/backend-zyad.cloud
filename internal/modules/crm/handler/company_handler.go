package handler

import (
	"net/http"
	"strings"
	"unicode/utf8"

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

type CompanyHandler struct {
	svc  service.CompanyService
	perm permissionmiddleware.CombinedPermissionChecker
}

// linkWorkspacePermission mengizinkan mengisi, mengganti, atau melepas tautan company ke workspace.
const linkWorkspacePermission = "company.link_workspace"

func NewCompanyHandler(svc service.CompanyService) *CompanyHandler {
	return &CompanyHandler{svc: svc}
}

// RegisterRoutes registers company routes under the given parent group. The
// parent is expected to already carry the CRM tenant/entitlement guards
// (RequireActiveTenant + RequireCustomerTenant + RequireEntitlement("crm.enabled")),
// set up once in internal/app/router.go for the whole /app/crm group.
func (h *CompanyHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	h.perm = p
	group := router.Group("/companies")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "company.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "company.create"), h.Create)
	group.GET("/lookup", permissionmiddleware.RequireOrganizationOrGlobal(p, "company.read"), h.Lookup)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "company.read"), h.Get)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "company.update"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "company.delete"), h.Delete)
	group.POST("/:id/restore", permissionmiddleware.RequireOrganizationOrGlobal(p, "company.restore"), h.Restore)
}

func (h *CompanyHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var query dto.CompanyListQuery
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

	companies, total, err := h.svc.List(c.Request.Context(), scope, repository.CompanyListFilter{
		Search:      query.Search,
		OwnerUserID: query.OwnerUserID,
		Limit:       perPage,
		Offset:      (page - 1) * perPage,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusOK, "success", dto.CompanyListFromDomain(companies), dto.BuildMeta(page, perPage, total))
}

func (h *CompanyHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.CreateCompanyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	company, err := h.svc.Create(c.Request.Context(), scope, repository.CreateCompanyParams{
		Name:        req.Name,
		Industry:    req.Industry,
		Website:     req.Website,
		Phone:       req.Phone,
		Email:       req.Email,
		Address:     req.Address,
		SizeRange:   req.SizeRange,
		Notes:       req.Notes,
		Tags:        req.Tags,
		OwnerUserID: req.OwnerUserID,
		CreatedBy:   userID,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.CompanyFromDomain(company))
}

func (h *CompanyHandler) Get(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	company, err := h.svc.Get(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", h.companyResponse(c, company))
}

func (h *CompanyHandler) Update(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	var req dto.UpdateCompanyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	// Tautan workspace diproses lebih dulu: pelanggaran (bukan platform, tanpa izin, workspace bukan
	// pelanggan, sudah tertaut ke company lain) menggagalkan request sebelum field lain diubah.
	var company domain.Company
	linked := false
	if req.TenantOrganizationID.Set {
		if err := h.authorizeWorkspaceLink(c); err != nil {
			corehttp.Fail(c, err)
			return
		}
		company, err = h.svc.SetWorkspaceLink(c.Request.Context(), scope, c.Param("id"), req.TenantOrganizationID.Value, userID)
		if err != nil {
			corehttp.Fail(c, err)
			return
		}
		linked = true
	}
	if !linked || hasCompanyFieldUpdates(req) {
		company, err = h.svc.Update(c.Request.Context(), scope, c.Param("id"), repository.UpdateCompanyParams{
			Name:        req.Name,
			Industry:    req.Industry,
			Website:     req.Website,
			Phone:       req.Phone,
			Email:       req.Email,
			Address:     req.Address,
			SizeRange:   req.SizeRange,
			Notes:       req.Notes,
			Tags:        req.Tags,
			OwnerUserID: req.OwnerUserID,
			UpdatedBy:   userID,
		})
		if err != nil {
			corehttp.Fail(c, err)
			return
		}
	}

	corehttp.OK(c, "success", h.companyResponse(c, company))
}

func hasCompanyFieldUpdates(req dto.UpdateCompanyRequest) bool {
	return req.Name != nil || req.Industry != nil || req.Website != nil || req.Phone != nil || req.Email != nil ||
		req.Address != nil || req.SizeRange != nil || req.Notes != nil || req.Tags != nil || req.OwnerUserID != nil
}

// authorizeWorkspaceLink: hanya org platform (422 untuk org lain) dan pemegang company.link_workspace (403).
func (h *CompanyHandler) authorizeWorkspaceLink(c *gin.Context) error {
	tenantContext, ok := coretenant.FromContext(c.Request.Context())
	if !ok {
		return coreerrors.New("TENANT_CONTEXT_REQUIRED", "organization context is required", http.StatusForbidden)
	}
	if tenantContext.OrganizationType() != coretenant.OrganizationTypePlatform {
		return coreerrors.New("WORKSPACE_LINK_PLATFORM_ONLY", "only the platform organization can link a workspace", http.StatusUnprocessableEntity)
	}
	if h.perm == nil {
		return nil
	}
	userID := permissionmiddleware.UserID(c)
	required := []string{linkWorkspacePermission}
	if err := h.perm.CanOrganization(c.Request.Context(), userID, tenantContext.OrganizationID(), required); err == nil {
		return nil
	}
	if err := h.perm.Can(c.Request.Context(), userID, required); err == nil {
		return nil
	}
	return coreerrors.New("FORBIDDEN", "insufficient permissions", http.StatusForbidden)
}

// companyResponse menambahkan ringkasan workspace tertaut; org non-platform tidak melihatnya.
func (h *CompanyHandler) companyResponse(c *gin.Context, company domain.Company) dto.CompanyResponse {
	resp := dto.CompanyFromDomain(company)
	if company.TenantOrganizationID == nil {
		return resp
	}
	if tenantContext, ok := coretenant.FromContext(c.Request.Context()); !ok || tenantContext.OrganizationType() != coretenant.OrganizationTypePlatform {
		return resp
	}
	if info := h.svc.Workspace(c.Request.Context(), *company.TenantOrganizationID); info != nil {
		resp.TenantOrganization = &dto.WorkspaceSummary{ID: info.ID, Name: info.Name, Slug: info.Slug, Status: info.Status}
	}
	return resp
}

func (h *CompanyHandler) Delete(c *gin.Context) {
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

func (h *CompanyHandler) Restore(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	userID := permissionmiddleware.UserID(c)

	if err := h.svc.Restore(c.Request.Context(), scope, c.Param("id"), userID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "restored", nil)
}

func (h *CompanyHandler) Lookup(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}
	name := strings.TrimSpace(c.Query("name"))
	if utf8.RuneCountInString(name) > 200 {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "name is too long", http.StatusUnprocessableEntity))
		return
	}
	companies, err := h.svc.FindSimilar(c.Request.Context(), scope, name)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.CompanyListFromDomain(companies))
}
