package handler

import (
	"errors"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/catalog/dto"
	"zyad.cloud/internal/modules/catalog/service"
)

type CategoryHandler struct {
	svc service.CategoryService
}

func NewCategoryHandler(svc service.CategoryService) *CategoryHandler {
	return &CategoryHandler{svc: svc}
}

func (h *CategoryHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/categories")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "catalog_product.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "catalog_product.create"), h.Create)
	group.PATCH("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "catalog_product.update"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "catalog_product.delete"), h.Delete)
}

func (h *CategoryHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		missingScope(c)
		return
	}
	categories, err := h.svc.List(c.Request.Context(), scope)
	if err != nil {
		failCatalog(c, err)
		return
	}
	items := make([]dto.CategoryResponse, 0, len(categories))
	for _, cat := range categories {
		items = append(items, dto.CategoryFromDomain(cat))
	}
	corehttp.OK(c, "success", items)
}

func (h *CategoryHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		missingScope(c)
		return
	}
	var req dto.CategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationError(c, err)
		return
	}
	if req.Name == nil {
		validationError(c, errors.New("name is required"))
		return
	}
	position := 0
	if req.Position != nil {
		position = *req.Position
	}
	category, err := h.svc.Create(c.Request.Context(), scope, *req.Name, position, permissionmiddleware.UserID(c))
	if err != nil {
		failCatalog(c, err)
		return
	}
	corehttp.OK(c, "success", dto.CategoryFromDomain(category))
}

func (h *CategoryHandler) Update(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		missingScope(c)
		return
	}
	var req dto.CategoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationError(c, err)
		return
	}
	category, err := h.svc.Update(c.Request.Context(), scope, c.Param("id"), req.Name, req.Position, permissionmiddleware.UserID(c))
	if err != nil {
		failCatalog(c, err)
		return
	}
	corehttp.OK(c, "success", dto.CategoryFromDomain(category))
}

func (h *CategoryHandler) Delete(c *gin.Context) {
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
