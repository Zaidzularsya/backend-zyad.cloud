package handler

import (
	"net/http"
	"strconv"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/dto"
	"zyad.cloud/internal/core/notification/repository"
	"zyad.cloud/internal/core/notification/service"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type TemplateHandler struct {
	service *service.TemplateService
	checker permissionmiddleware.PermissionChecker
}

func NewTemplateHandler(templateService *service.TemplateService, checker permissionmiddleware.PermissionChecker) *TemplateHandler {
	return &TemplateHandler{
		service: templateService,
		checker: checker,
	}
}

func (h *TemplateHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/admin/notification-templates")
	group.GET("", h.require("notification_template.read"), h.ListTemplates)
	group.GET("/:id", h.require("notification_template.read"), h.GetTemplate)
	group.POST("", h.require("notification_template.create"), h.CreateTemplate)
	group.PATCH("/:id", h.require("notification_template.update"), h.UpdateTemplate)
	group.DELETE("/:id", h.require("notification_template.delete"), h.DeleteTemplate)
	group.POST("/:id/preview", h.require("notification_template.preview"), h.PreviewTemplate)
	group.POST("/:id/activate", h.require("notification_template.activate"), h.ActivateTemplate)
	group.POST("/:id/deactivate", h.require("notification_template.deactivate"), h.DeactivateTemplate)
	group.POST("/:id/archive", h.require("notification_template.archive"), h.ArchiveTemplate)
	group.POST("/:id/clone", h.require("notification_template.clone"), h.CloneTemplate)
}

func (h *TemplateHandler) ListTemplates(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	offset := queryInt(c, "offset", 0)
	filter := repository.TemplateListFilter{
		Code:    c.Query("code"),
		Channel: domain.Channel(c.Query("channel")),
		Locale:  c.Query("locale"),
		Status:  domain.TemplateStatus(c.Query("status")),
		Limit:   limit,
		Offset:  offset,
	}
	if value, ok := queryBool(c, "is_active"); ok {
		filter.IsActive = &value
	}

	templates, err := h.service.ListTemplates(c.Request.Context(), filter)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusOK, "notification templates retrieved", dto.NewTemplateResponses(templates), gin.H{
		"limit":  limit,
		"offset": offset,
		"count":  len(templates),
	})
}

func (h *TemplateHandler) GetTemplate(c *gin.Context) {
	template, err := h.service.GetTemplate(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "notification template retrieved", dto.NewTemplateResponse(template))
}

func (h *TemplateHandler) CreateTemplate(c *gin.Context) {
	var req dto.CreateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	template, err := h.service.CreateTemplate(c.Request.Context(), req)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.Created(c, "notification template created", dto.NewTemplateResponse(template))
}

func (h *TemplateHandler) UpdateTemplate(c *gin.Context) {
	var req dto.UpdateTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	template, err := h.service.UpdateTemplate(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "notification template updated", dto.NewTemplateResponse(template))
}

func (h *TemplateHandler) DeleteTemplate(c *gin.Context) {
	if err := h.service.DeleteTemplate(c.Request.Context(), c.Param("id")); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "notification template deleted", nil)
}

func (h *TemplateHandler) PreviewTemplate(c *gin.Context) {
	var req dto.PreviewTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	rendered, err := h.service.PreviewTemplate(c.Request.Context(), c.Param("id"), req.Payload)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "notification template preview rendered", dto.PreviewTemplateResponse{
		Subject: rendered.Subject,
		Body:    rendered.Body,
		Payload: req.Payload,
	})
}

func (h *TemplateHandler) ActivateTemplate(c *gin.Context) {
	if err := h.service.ActivateTemplate(c.Request.Context(), c.Param("id")); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "notification template activated", nil)
}

func (h *TemplateHandler) DeactivateTemplate(c *gin.Context) {
	if err := h.service.DeactivateTemplate(c.Request.Context(), c.Param("id")); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "notification template deactivated", nil)
}

func (h *TemplateHandler) ArchiveTemplate(c *gin.Context) {
	if err := h.service.ArchiveTemplate(c.Request.Context(), c.Param("id")); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "notification template archived", nil)
}

func (h *TemplateHandler) CloneTemplate(c *gin.Context) {
	template, err := h.service.CloneTemplate(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.Created(c, "notification template cloned", dto.NewTemplateResponse(template))
}

func (h *TemplateHandler) require(permission string) gin.HandlerFunc {
	return permissionmiddleware.Require(h.checker, permission)
}

func queryInt(c *gin.Context, key string, fallback int) int {
	value := c.Query(key)
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func queryBool(c *gin.Context, key string) (bool, bool) {
	value := c.Query(key)
	if value == "" {
		return false, false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, false
	}
	return parsed, true
}
