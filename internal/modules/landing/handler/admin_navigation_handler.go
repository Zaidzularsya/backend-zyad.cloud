package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/dto"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
)

type AdminNavigationHandler struct {
	navigationService service.NavigationService
}

func NewAdminNavigationHandler(navigationService service.NavigationService) *AdminNavigationHandler {
	return &AdminNavigationHandler{
		navigationService: navigationService,
	}
}

func (h *AdminNavigationHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.PermissionChecker) {
	group := router.Group("/admin/landing/menus")

	group.GET("", permissionmiddleware.Require(p, "landing.menu.manage"), h.ListMenus)
	group.POST("", permissionmiddleware.Require(p, "landing.menu.manage"), h.CreateMenu)
	group.PATCH("/:id", permissionmiddleware.Require(p, "landing.menu.manage"), h.UpdateMenu)
	group.DELETE("/:id", permissionmiddleware.Require(p, "landing.menu.manage"), h.DeleteMenu)

	// Items
	group.GET("/:id/items", permissionmiddleware.Require(p, "landing.menu.manage"), h.ListMenuItems)
	group.POST("/:id/items", permissionmiddleware.Require(p, "landing.menu.manage"), h.CreateMenuItem)
	group.PATCH("/:id/items/:itemId", permissionmiddleware.Require(p, "landing.menu.manage"), h.UpdateMenuItem)
	group.DELETE("/:id/items/:itemId", permissionmiddleware.Require(p, "landing.menu.manage"), h.DeleteMenuItem)
	group.PUT("/:id/items/reorder", permissionmiddleware.Require(p, "landing.menu.manage"), h.ReorderMenuItems)
}

func (h *AdminNavigationHandler) ListMenus(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	menus, err := h.navigationService.ListMenus(c.Request.Context(), scope)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", menus)
}

func (h *AdminNavigationHandler) CreateMenu(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)

	var req dto.MenuRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := repository.CreateMenuParams{
		Name:      req.Name,
		Location:  domain.MenuLocation(req.Location),
		IsActive:  req.IsActive,
		CreatedBy: userID,
	}

	menu, err := h.navigationService.CreateMenu(c.Request.Context(), scope, params)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", menu)
}

func (h *AdminNavigationHandler) UpdateMenu(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)

	id := c.Param("id")

	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := repository.UpdateMenuParams{
		UpdatedBy: userID,
	}

	if v, ok := req["name"].(string); ok {
		params.Name = &v
	}
	if v, ok := req["location"].(string); ok {
		loc := domain.MenuLocation(v)
		params.Location = &loc
	}
	if v, ok := req["is_active"].(bool); ok {
		params.IsActive = &v
	}

	menu, err := h.navigationService.UpdateMenu(c.Request.Context(), scope, id, params)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", menu)
}

func (h *AdminNavigationHandler) DeleteMenu(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)

	id := c.Param("id")

	if err := h.navigationService.DeleteMenu(c.Request.Context(), scope, id, userID); err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "deleted", nil)
}

func (h *AdminNavigationHandler) ListMenuItems(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	menuID := c.Param("id")

	items, err := h.navigationService.ListMenuItems(c.Request.Context(), scope, menuID)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", items)
}

func (h *AdminNavigationHandler) CreateMenuItem(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	_ = permissionmiddleware.UserID(c)

	menuID := c.Param("id")

	var req dto.MenuItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := repository.CreateMenuItemParams{
		MenuID:      menuID,
		ParentID:    req.ParentID,
		Label:       req.Label,
		LinkType:    domain.LinkType(req.LinkType),
		Destination: req.Destination,
		Target:      domain.CTATarget(req.Target),
		SortOrder:   req.SortOrder,
		IsEnabled:   req.IsEnabled,
	}

	item, err := h.navigationService.CreateMenuItem(c.Request.Context(), scope, params)
	if err != nil {
		if err == service.ErrMaxMenuDepth {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusBadRequest))
			return
		}
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", item)
}

func (h *AdminNavigationHandler) UpdateMenuItem(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	_ = permissionmiddleware.UserID(c)

	itemID := c.Param("itemId")

	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := repository.UpdateMenuItemParams{}

	if v, exists := req["parent_id"]; exists {
		if parentID, ok := v.(string); ok {
			params.ParentID = &parentID
		} else if v == nil {
			parentID := ""
			params.ParentID = &parentID
		}
	}
	if v, ok := req["label"].(string); ok {
		params.Label = &v
	}
	if v, ok := req["link_type"].(string); ok {
		lt := domain.LinkType(v)
		params.LinkType = &lt
	}
	if v, ok := req["destination"].(string); ok {
		params.Destination = &v
	}
	if v, ok := req["target"].(string); ok {
		t := domain.CTATarget(v)
		params.Target = &t
	}
	if v, ok := req["sort_order"].(float64); ok {
		sortOrder := int(v)
		params.SortOrder = &sortOrder
	}
	if v, ok := req["is_enabled"].(bool); ok {
		params.IsEnabled = &v
	}

	item, err := h.navigationService.UpdateMenuItem(c.Request.Context(), scope, itemID, params)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", item)
}

func (h *AdminNavigationHandler) DeleteMenuItem(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	itemID := c.Param("itemId")

	if err := h.navigationService.DeleteMenuItem(c.Request.Context(), scope, itemID); err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "deleted", nil)
}

func (h *AdminNavigationHandler) ReorderMenuItems(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	menuID := c.Param("id")

	var req struct {
		ItemIDs []string `json:"item_ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	if err := h.navigationService.ReorderMenuItems(c.Request.Context(), scope, menuID, req.ItemIDs); err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "reordered", nil)
}
