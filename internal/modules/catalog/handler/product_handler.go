package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/catalog/dto"
	"zyad.cloud/internal/modules/catalog/repository"
	"zyad.cloud/internal/modules/catalog/service"
	"zyad.cloud/internal/shared/response"
)

type ProductHandler struct {
	svc service.ProductService
}

func NewProductHandler(svc service.ProductService) *ProductHandler {
	return &ProductHandler{svc: svc}
}

// RegisterRoutes registers product routes under the given parent group; the
// parent applies the tenant & CRM entitlement guards.
func (h *ProductHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/products")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "catalog_product.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "catalog_product.create"), h.Create)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "catalog_product.read"), h.Get)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "catalog_product.update"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "catalog_product.delete"), h.Delete)
}

func (h *ProductHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		missingScope(c)
		return
	}
	var query dto.ProductListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		validationError(c, err)
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
	if perPage > 100 {
		perPage = 100
	}
	var isActive *bool
	if query.IsActive != "" {
		v := query.IsActive == "true"
		isActive = &v
	}

	products, total, err := h.svc.List(c.Request.Context(), scope, repository.ProductListFilter{
		Search:     query.Q,
		CategoryID: query.CategoryID,
		IsActive:   isActive,
		Limit:      perPage,
		Offset:     (page - 1) * perPage,
	})
	if err != nil {
		failCatalog(c, err)
		return
	}
	items := make([]dto.ProductResponse, 0, len(products))
	for _, p := range products {
		items = append(items, dto.ProductFromDomain(p))
	}
	response.JSON(c, http.StatusOK, "success", items, dto.BuildMeta(page, perPage, total))
}

func (h *ProductHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		missingScope(c)
		return
	}
	var req dto.CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationError(c, err)
		return
	}
	product, err := h.svc.Create(c.Request.Context(), scope, service.ProductInput{
		CategoryID:  req.CategoryID,
		SKU:         req.SKU,
		Name:        req.Name,
		Description: req.Description,
		Unit:        req.Unit,
		BasePrice:   req.BasePrice,
		TaxPercent:  req.TaxPercent,
		IsActive:    req.IsActive == nil || *req.IsActive,
	}, permissionmiddleware.UserID(c))
	if err != nil {
		failCatalog(c, err)
		return
	}
	corehttp.OK(c, "success", dto.ProductFromDomain(product))
}

func (h *ProductHandler) Get(c *gin.Context) {
	h.respond(c, func(scope coretenant.Scope) (domain.Product, error) {
		return h.svc.Get(c.Request.Context(), scope, c.Param("id"))
	})
}

func (h *ProductHandler) Update(c *gin.Context) {
	var req dto.UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationError(c, err)
		return
	}
	h.respond(c, func(scope coretenant.Scope) (domain.Product, error) {
		return h.svc.Update(c.Request.Context(), scope, c.Param("id"), repository.UpdateProductParams{
			CategoryID:  req.CategoryID,
			SKU:         req.SKU,
			Name:        req.Name,
			Description: req.Description,
			Unit:        req.Unit,
			BasePrice:   req.BasePrice,
			TaxPercent:  req.TaxPercent,
			IsActive:    req.IsActive,
			UpdatedBy:   permissionmiddleware.UserID(c),
		})
	})
}

func (h *ProductHandler) Delete(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		missingScope(c)
		return
	}
	if err := h.svc.Delete(c.Request.Context(), scope, c.Param("id"), permissionmiddleware.UserID(c)); err != nil {
		failCatalog(c, err)
		return
	}
	corehttp.OK(c, "deleted", nil)
}

func (h *ProductHandler) respond(c *gin.Context, fn func(coretenant.Scope) (domain.Product, error)) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		missingScope(c)
		return
	}
	product, err := fn(scope)
	if err != nil {
		failCatalog(c, err)
		return
	}
	corehttp.OK(c, "success", dto.ProductFromDomain(product))
}
