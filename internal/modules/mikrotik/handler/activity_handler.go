package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/dto"
	"zyad.cloud/internal/modules/mikrotik/service"
	"zyad.cloud/internal/shared/response"
)

// ActivityService is the subset of service.ActivityService used by the handler.
type ActivityService interface {
	List(ctx context.Context, scope coretenant.Scope, limit, offset int) ([]service.ActivityItem, int64, error)
}

// ActivityHandler serves the control center activity log (read-only).
type ActivityHandler struct{ svc ActivityService }

func NewActivityHandler(svc ActivityService) *ActivityHandler { return &ActivityHandler{svc: svc} }

const (
	defaultActivityPage = 50
	maxActivityPage     = 100
)

// RegisterRoutes mounts GET /control/activity. Nothing here contacts a router.
func (h *ActivityHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker, _ gin.HandlerFunc) {
	router.GET("/control/activity", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_control.read"), h.List)
}

func (h *ActivityHandler) List(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var q dto.ActivityQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		validationFail(c, err)
		return
	}
	limit, offset := q.Limit, q.Offset
	if limit <= 0 {
		limit = defaultActivityPage
	}
	if limit > maxActivityPage {
		limit = maxActivityPage
	}
	if offset < 0 {
		offset = 0
	}
	items, total, err := h.svc.List(c.Request.Context(), scope, limit, offset)
	if err != nil {
		failRouter(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "success", dto.ActivityListFromService(items),
		dto.ActivityMeta{Limit: limit, Offset: offset, Total: total})
}
