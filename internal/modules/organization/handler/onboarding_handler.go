package handler

import (
	"context"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/service"

	"github.com/gin-gonic/gin"
)

type OnboardingService interface {
	CreateWorkspace(
		context.Context,
		string,
		string,
		dto.CreateWorkspaceRequest,
		service.OnboardingMetadata,
	) (dto.CreateWorkspaceResponse, error)
}

type OnboardingHandler struct {
	service OnboardingService
}

func NewOnboardingHandler(service OnboardingService) *OnboardingHandler {
	return &OnboardingHandler{service: service}
}

func (h *OnboardingHandler) RegisterRoutes(router gin.IRoutes) {
	router.POST("/onboarding/workspace", h.CreateWorkspace)
}

func (h *OnboardingHandler) CreateWorkspace(c *gin.Context) {
	user, ok := middleware.AuthenticatedUserFromContext(c)
	if !ok || user.ID == "" || user.SessionID == "" {
		corehttp.Fail(c, coreerrors.New(
			"UNAUTHORIZED",
			"authenticated session is required",
			http.StatusUnauthorized,
		))
		return
	}

	var request dto.CreateWorkspaceRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}

	result, err := h.service.CreateWorkspace(
		c.Request.Context(),
		user.ID,
		user.SessionID,
		request,
		service.OnboardingMetadata{
			RequestID: middleware.RequestIDFromContext(c),
			IPAddress: c.ClientIP(),
			UserAgent: c.Request.UserAgent(),
		},
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "workspace created successfully", result)
}
