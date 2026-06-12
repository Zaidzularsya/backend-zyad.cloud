package handler

import (
	"context"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/dto"
	"zyad.cloud/internal/core/notification/repository"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

type NotificationLogService interface {
	ListLogs(ctx context.Context, filter repository.NotificationLogListFilter) ([]domain.NotificationLog, error)
	GetLog(ctx context.Context, logID string) (domain.NotificationLog, error)
	Retry(ctx context.Context, logID string) (domain.NotificationLog, error)
	Cancel(ctx context.Context, logID string) (domain.NotificationLog, error)
}

type LogHandler struct {
	service NotificationLogService
	checker permissionmiddleware.PermissionChecker
}

func NewLogHandler(notificationService NotificationLogService, checker permissionmiddleware.PermissionChecker) *LogHandler {
	return &LogHandler{
		service: notificationService,
		checker: checker,
	}
}

func (h *LogHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("/admin/notification-logs")
	group.GET("", h.require("notification_log.read"), h.ListLogs)
	group.GET("/:id", h.require("notification_log.read"), h.GetLog)
	group.POST("/:id/retry", h.require("notification_log.retry"), h.RetryLog)
	group.POST("/:id/cancel", h.require("notification_log.cancel"), h.CancelLog)
}

func (h *LogHandler) ListLogs(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	offset := queryInt(c, "offset", 0)
	filter := repository.NotificationLogListFilter{
		EventType:       c.Query("event_type"),
		TemplateCode:    c.Query("template_code"),
		OrganizationID:  c.Query("organization_id"),
		RecipientUserID: c.Query("recipient_user_id"),
		Channel:         domain.Channel(c.Query("channel")),
		Status:          domain.LogStatus(c.Query("status")),
		Limit:           limit,
		Offset:          offset,
	}

	logs, err := h.service.ListLogs(c.Request.Context(), filter)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusOK, "notification logs retrieved", dto.NewNotificationLogResponses(logs), gin.H{
		"limit":  limit,
		"offset": offset,
		"count":  len(logs),
	})
}

func (h *LogHandler) GetLog(c *gin.Context) {
	log, err := h.service.GetLog(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "notification log retrieved", dto.NewNotificationLogResponse(log))
}

func (h *LogHandler) RetryLog(c *gin.Context) {
	log, err := h.service.Retry(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "notification log retried", dto.NewNotificationLogResponse(log))
}

func (h *LogHandler) CancelLog(c *gin.Context) {
	log, err := h.service.Cancel(c.Request.Context(), c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "notification log cancelled", dto.NewNotificationLogResponse(log))
}

func (h *LogHandler) require(permission string) gin.HandlerFunc {
	if h.service == nil {
		return func(c *gin.Context) {
			corehttp.Fail(c, coreerrors.New("NOTIFICATION_LOG_SERVICE_REQUIRED", "notification log service is required", http.StatusInternalServerError))
			c.Abort()
		}
	}
	return permissionmiddleware.Require(h.checker, permission)
}
