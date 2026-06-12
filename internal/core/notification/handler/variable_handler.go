package handler

import (
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/notification/dto"
	notificationtemplate "zyad.cloud/internal/core/notification/template"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"

	"github.com/gin-gonic/gin"
)

type VariableHandler struct {
	registry *notificationtemplate.VariableRegistry
	checker  permissionmiddleware.PermissionChecker
}

func NewVariableHandler(registry *notificationtemplate.VariableRegistry, checker permissionmiddleware.PermissionChecker) *VariableHandler {
	if registry == nil {
		registry = notificationtemplate.NewVariableRegistry()
	}
	return &VariableHandler{
		registry: registry,
		checker:  checker,
	}
}

func (h *VariableHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/admin/notification-template-variables")
	group.GET("", h.require("notification_variable.read"), h.ListVariables)
	group.GET("/:code", h.require("notification_variable.read"), h.GetVariables)
}

func (h *VariableHandler) ListVariables(c *gin.Context) {
	result := make(map[string][]dto.VariableItem)
	for _, code := range h.registry.Codes() {
		result[code] = dto.NewVariableItems(h.registry.Get(code))
	}

	corehttp.OK(c, "notification template variables retrieved", result)
}

func (h *VariableHandler) GetVariables(c *gin.Context) {
	code := c.Param("code")
	variables := h.registry.Get(code)
	if len(variables) == 0 {
		corehttp.Fail(c, coreerrors.New("NOTIFICATION_VARIABLES_NOT_FOUND", "notification template variables not found", http.StatusNotFound))
		return
	}

	corehttp.OK(c, "notification template variables retrieved", dto.NewVariableItems(variables))
}

func (h *VariableHandler) require(permission string) gin.HandlerFunc {
	return permissionmiddleware.Require(h.checker, permission)
}
