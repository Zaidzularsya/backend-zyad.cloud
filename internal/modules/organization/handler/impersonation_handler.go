package handler

import (
	"context"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/organization/dto"
	"zyad.cloud/internal/modules/organization/service"

	"github.com/gin-gonic/gin"
)

type OrganizationImpersonationService interface {
	Start(
		context.Context,
		string,
		string,
		string,
		dto.StartImpersonationRequest,
		service.ImpersonationMetadata,
	) (dto.ImpersonationSessionResponse, error)
	Stop(
		context.Context,
		string,
		string,
		dto.StopImpersonationRequest,
		service.ImpersonationMetadata,
	) (dto.ImpersonationSessionResponse, error)
}

type ImpersonationHandler struct {
	service OrganizationImpersonationService
	checker permissionmiddleware.PermissionChecker
}

func NewImpersonationHandler(
	service OrganizationImpersonationService,
	checker permissionmiddleware.PermissionChecker,
) *ImpersonationHandler {
	return &ImpersonationHandler{service: service, checker: checker}
}

func (h *ImpersonationHandler) RegisterRoutes(router *gin.RouterGroup) {
	group := router.Group("")
	group.Use(middleware.RequirePlatformTenant())
	group.POST(
		"/platform/organizations/:id/impersonate",
		permissionmiddleware.Require(h.checker, "platform.organization.impersonate"),
		h.Start,
	)
	group.DELETE(
		"/platform/impersonation",
		permissionmiddleware.Require(h.checker, "platform.organization.impersonate"),
		h.Stop,
	)
}

func (h *ImpersonationHandler) Start(c *gin.Context) {
	user, ok := middleware.AuthenticatedUserFromContext(c)
	if !ok || user.ID == "" || user.SessionID == "" {
		corehttp.Fail(c, authenticatedSessionRequiredError())
		return
	}
	var request dto.StartImpersonationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	session, err := h.service.Start(
		c.Request.Context(),
		user.ID,
		user.SessionID,
		c.Param("id"),
		request,
		impersonationMetadata(c),
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "organization impersonation started successfully", session)
}

func (h *ImpersonationHandler) Stop(c *gin.Context) {
	user, ok := middleware.AuthenticatedUserFromContext(c)
	if !ok || user.ID == "" || user.SessionID == "" {
		corehttp.Fail(c, authenticatedSessionRequiredError())
		return
	}
	var request dto.StopImpersonationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		corehttp.Fail(c, validationHandlerError(err))
		return
	}
	session, err := h.service.Stop(
		c.Request.Context(),
		user.ID,
		user.SessionID,
		request,
		impersonationMetadata(c),
	)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "organization impersonation stopped successfully", session)
}

func impersonationMetadata(c *gin.Context) service.ImpersonationMetadata {
	return service.ImpersonationMetadata{
		RequestID: middleware.RequestIDFromContext(c),
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}
}

func authenticatedSessionRequiredError() error {
	return coreerrors.New(
		"UNAUTHORIZED",
		"authenticated session is required",
		http.StatusUnauthorized,
	)
}
