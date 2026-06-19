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
	"zyad.cloud/internal/shared/response"
)

type AdminSectionHandler struct {
	sectionSvc service.SectionService
}

func NewAdminSectionHandler(sectionSvc service.SectionService) *AdminSectionHandler {
	return &AdminSectionHandler{
		sectionSvc: sectionSvc,
	}
}

func (h *AdminSectionHandler) RegisterRoutes(router *gin.RouterGroup, checker permissionmiddleware.PermissionChecker) {
	group := router.Group("/admin/landing-pages/:id/sections")
	
	group.GET("", permissionmiddleware.Require(checker, "landing.page.read"), h.ListSections)
	group.POST("", permissionmiddleware.Require(checker, "landing.section.manage"), h.CreateSection)
	group.PATCH("/:sectionId", permissionmiddleware.Require(checker, "landing.section.manage"), h.UpdateSection)
	group.DELETE("/:sectionId", permissionmiddleware.Require(checker, "landing.section.manage"), h.DeleteSection)
	group.PUT("/reorder", permissionmiddleware.Require(checker, "landing.section.manage"), h.ReorderSections)
}

// ListSections godoc
func (h *AdminSectionHandler) ListSections(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	sections, err := h.sectionSvc.ListByPage(c.Request.Context(), scope, pageID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusOK, "Sections retrieved successfully", sections, nil)
}

// CreateSection godoc
func (h *AdminSectionHandler) CreateSection(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	var req dto.CreateSectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := repository.CreateSectionParams{
		LandingPageID: pageID,
		Key:           req.Key,
		Type:          domain.SectionType(req.Type),
		Name:          req.Name,
		SortOrder:     req.SortOrder,
		IsEnabled:     req.IsEnabled,
		Content:       req.Content,
		Style:         req.Style,
	}

	section, err := h.sectionSvc.Create(c.Request.Context(), scope, params)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusCreated, "Section created successfully", section, nil)
}

// UpdateSection godoc
func (h *AdminSectionHandler) UpdateSection(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	sectionID := c.Param("sectionId")
	var req dto.UpdateSectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	var content map[string]any
	if req.Content != nil {
		content = *req.Content
	}
	var style map[string]any
	if req.Style != nil {
		style = *req.Style
	}

	params := repository.UpdateSectionParams{
		Name:      req.Name,
		IsEnabled: req.IsEnabled,
		Content:   content,
		Style:     style,
		UpdatedBy: permissionmiddleware.UserID(c),
	}

	section, err := h.sectionSvc.Update(c.Request.Context(), scope, sectionID, params)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Section updated successfully", section)
}

// DeleteSection godoc
func (h *AdminSectionHandler) DeleteSection(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	sectionID := c.Param("sectionId")
	if err := h.sectionSvc.Delete(c.Request.Context(), scope, sectionID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Section deleted successfully", nil)
}

// ReorderSections godoc
func (h *AdminSectionHandler) ReorderSections(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	var req dto.ReorderSectionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	var items []repository.SectionReorderParam
	for _, item := range req.Items {
		items = append(items, repository.SectionReorderParam{
			ID:        item.ID,
			SortOrder: item.SortOrder,
		})
	}

	if err := h.sectionSvc.Reorder(c.Request.Context(), scope, pageID, items); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Sections reordered successfully", nil)
}
