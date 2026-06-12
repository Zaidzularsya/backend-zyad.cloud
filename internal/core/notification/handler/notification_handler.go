package handler

import (
	"context"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/dto"

	"github.com/gin-gonic/gin"
)

type NotificationSender interface {
	SendByTemplate(ctx context.Context, notification domain.Notification) (domain.NotificationLog, error)
}

type NotificationHandler struct {
	service NotificationSender
}

func NewNotificationHandler(notificationService NotificationSender) *NotificationHandler {
	return &NotificationHandler{service: notificationService}
}

func (h *NotificationHandler) RegisterInternalRoutes(router *gin.RouterGroup) {
	group := router.Group("/internal/notifications")
	group.POST("/send", h.SendByTemplate)
}

func (h *NotificationHandler) SendByTemplate(c *gin.Context) {
	var req dto.SendNotificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	log, err := h.service.SendByTemplate(c.Request.Context(), domain.Notification{
		EventID:        req.EventID,
		EventType:      req.EventType,
		TemplateCode:   req.TemplateCode,
		OrganizationID: req.OrganizationID,
		Channel:        domain.Channel(req.Channel),
		UserID:         req.UserID,
		Recipient: domain.NotificationRecipient{
			Type:        req.Recipient.Type,
			UserID:      req.Recipient.UserID,
			Name:        req.Recipient.Name,
			Email:       req.Recipient.Email,
			Phone:       req.Recipient.Phone,
			Destination: req.Recipient.Destination,
		},
		Payload: req.Payload,
		Locale:  req.Locale,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.Created(c, "notification sent", dto.NewNotificationLogResponse(log))
}
