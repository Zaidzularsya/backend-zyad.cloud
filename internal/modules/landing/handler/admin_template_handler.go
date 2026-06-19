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

type AdminTemplateHandler struct {
	templateService service.TemplateService
}

func NewAdminTemplateHandler(templateService service.TemplateService) *AdminTemplateHandler {
	return &AdminTemplateHandler{
		templateService: templateService,
	}
}

func (h *AdminTemplateHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.PermissionChecker) {
	group := router.Group("/admin/landing/section-templates")

	group.GET("", permissionmiddleware.Require(p, "landing.section_template.manage"), h.List)
	group.POST("", permissionmiddleware.Require(p, "landing.section_template.manage"), h.Create)
	group.PATCH("/:id", permissionmiddleware.Require(p, "landing.section_template.manage"), h.Update)
	group.DELETE("/:id", permissionmiddleware.Require(p, "landing.section_template.manage"), h.Delete)

	// Instantiate template to a page
	router.POST("/admin/landing-pages/:id/sections/from-template", permissionmiddleware.Require(p, "landing.page.update"), h.InstantiateToPage)
}

func (h *AdminTemplateHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	sectionType := c.Query("section_type")

	templates, err := h.templateService.List(c.Request.Context(), scope, domain.SectionType(sectionType))
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", templates)
}

func (h *AdminTemplateHandler) Create(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)

	var req dto.SectionTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := repository.CreateSectionTemplateParams{
		Name:        req.Name,
		Description: req.Description,
		SectionType: domain.SectionType(req.SectionType),
		Content:     req.Content,
		Style:       req.Style,
		CreatedBy:   userID,
	}

	template, err := h.templateService.Create(c.Request.Context(), scope, params)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", template)
}

func (h *AdminTemplateHandler) Update(c *gin.Context) {
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

	params := repository.UpdateSectionTemplateParams{
		UpdatedBy: userID,
	}

	if v, ok := req["name"].(string); ok {
		params.Name = &v
	}
	if v, ok := req["description"].(string); ok {
		params.Description = &v
	}
	if v, ok := req["content"].(map[string]any); ok {
		params.Content = v
	}
	if v, ok := req["style"].(map[string]any); ok {
		params.Style = v
	}

	template, err := h.templateService.Update(c.Request.Context(), scope, id, params)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", template)
}

func (h *AdminTemplateHandler) Delete(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)

	id := c.Param("id")

	if err := h.templateService.Delete(c.Request.Context(), scope, id, userID); err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "deleted", nil)
}

func (h *AdminTemplateHandler) InstantiateToPage(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)

	pageID := c.Param("id")

	var req struct {
		TemplateID string `json:"template_id" binding:"required,uuid"`
		SectionKey string `json:"section_key" binding:"required,lowercase,alphanumhyphen"`
		SortOrder  int    `json:"sort_order"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	section, err := h.templateService.InstantiateToPage(c.Request.Context(), scope, req.TemplateID, pageID, req.SectionKey, req.SortOrder, userID)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", section)
}
