package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/dto"
	"zyad.cloud/internal/modules/mikrotik/repository"
	"zyad.cloud/internal/modules/mikrotik/service"
	"zyad.cloud/internal/shared/response"
)

// TaskService is the subset of service.TaskService used by the handler.
type TaskService interface {
	List(ctx context.Context, scope coretenant.Scope, f repository.TaskFilter, limit, offset int) ([]domain.TaskView, int64, error)
	Summary(ctx context.Context, scope coretenant.Scope) (service.TaskSummary, error)
	Retry(ctx context.Context, scope coretenant.Scope, id, actorUserID string) (domain.TaskView, error)
}

type TaskHandler struct {
	svc TaskService
}

func NewTaskHandler(svc TaskService) *TaskHandler { return &TaskHandler{svc: svc} }

// RegisterRoutes mounts the isolation task dashboard under the parent group.
// Retry shares the per-organization limiter of the other mutating routes.
func (h *TaskHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker, probeLimit gin.HandlerFunc) {
	limit := orPass(probeLimit)
	router.GET("/tasks", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_task.read"), h.List)
	router.GET("/tasks/summary", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_task.read"), h.Summary)
	router.POST("/tasks/:id/retry", limit, permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_task.retry"), h.Retry)
}

const defaultTaskPageSize = 50

var validTaskStatuses = map[string]bool{
	domain.TaskPending: true, domain.TaskRunning: true, domain.TaskDone: true, domain.TaskFailed: true, domain.TaskCancelled: true,
}

func (h *TaskHandler) List(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var query dto.TaskListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}
	if query.Status != "" && !validTaskStatuses[query.Status] {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "status must be one of pending, running, done, failed, cancelled")
		return
	}
	if query.RouterID != "" && !validUUID(query.RouterID) {
		response.Error(c, http.StatusBadRequest, "VALIDATION_ERROR", "router_id must be a uuid")
		return
	}
	limit, offset := query.Limit, query.Offset
	if limit <= 0 {
		limit = defaultTaskPageSize
	}
	if limit > domain.MaxRouterPageSize {
		// Same clamp as the repository, so meta.per_page is the real page size.
		limit = domain.MaxRouterPageSize
	}
	if offset < 0 {
		offset = 0
	}
	tasks, total, err := h.svc.List(c.Request.Context(), scope,
		repository.TaskFilter{Status: query.Status, RouterID: query.RouterID}, limit, offset)
	if err != nil {
		failTask(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "success", dto.TaskListFromView(tasks), dto.BuildMeta(offset/limit+1, limit, total))
}

func (h *TaskHandler) Summary(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	sum, err := h.svc.Summary(c.Request.Context(), scope)
	if err != nil {
		failTask(c, err)
		return
	}
	corehttp.OK(c, "success", dto.TaskSummaryFromService(sum))
}

func (h *TaskHandler) Retry(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if !validUUID(id) {
		failTask(c, mikrotik.ErrTaskNotFound)
		return
	}
	view, err := h.svc.Retry(c.Request.Context(), scope, id, permissionmiddleware.UserID(c))
	if err != nil {
		failTask(c, err)
		return
	}
	corehttp.OK(c, "success", dto.TaskFromView(view))
}

func failTask(c *gin.Context, err error) {
	switch {
	case errors.Is(err, mikrotik.ErrTaskNotFound):
		response.Error(c, http.StatusNotFound, "MIKROTIK_TASK_NOT_FOUND", "isolation task not found")
	case errors.Is(err, mikrotik.ErrTaskNotRetryable):
		response.Error(c, http.StatusConflict, "MIKROTIK_TASK_NOT_RETRYABLE", "only a failed task can be retried")
	case errors.Is(err, mikrotik.ErrTaskActiveExists):
		response.Error(c, http.StatusConflict, "MIKROTIK_TASK_ACTIVE_EXISTS", "an active task already exists for this secret, wait for it to finish")
	default:
		failRouter(c, err)
	}
}
