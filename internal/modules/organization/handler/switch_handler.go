package handler

import (
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/service"

	"github.com/gin-gonic/gin"
)

type SwitchHandler struct {
	service *service.SwitchService
}

func NewSwitchHandler(switchService *service.SwitchService) *SwitchHandler {
	return &SwitchHandler{service: switchService}
}

func (h *SwitchHandler) RegisterRoutes(router gin.IRoutes) {
	router.GET("/users/me/organizations", h.List)
	router.POST("/users/me/switch-organization", h.Switch)
}

func (h *SwitchHandler) List(c *gin.Context) {
	user, ok := middleware.AuthenticatedUserFromContext(c)
	if !ok || user.ID == "" || user.SessionID == "" {
		corehttp.Fail(c, coreerrors.New(
			"UNAUTHORIZED",
			"authenticated session is required",
			http.StatusUnauthorized,
		))
		return
	}
	results, err := h.service.List(c.Request.Context(), user.ID, user.SessionID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "organizations retrieved successfully", results)
}

func (h *SwitchHandler) Switch(c *gin.Context) {
	user, ok := middleware.AuthenticatedUserFromContext(c)
	if !ok || user.ID == "" || user.SessionID == "" {
		corehttp.Fail(c, coreerrors.New(
			"UNAUTHORIZED",
			"authenticated session is required",
			http.StatusUnauthorized,
		))
		return
	}
	var request dto.SwitchOrganizationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, coreerrors.New(
			"VALIDATION_ERROR",
			err.Error(),
			http.StatusUnprocessableEntity,
		))
		return
	}
	result, err := h.service.Switch(
		c.Request.Context(),
		user.ID,
		user.SessionID,
		request,
		service.SwitchMetadata{
			RequestID: middleware.RequestIDFromContext(c),
			IPAddress: c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
		},
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "organization switched successfully", result)
}
