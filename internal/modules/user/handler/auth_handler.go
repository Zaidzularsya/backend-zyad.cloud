package handler

import (
	"strings"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/modules/user/dto"
	"zyad.cloud/internal/modules/user/service"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	service *service.AuthService
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{service: authService}
}

func (h *AuthHandler) RegisterRoutes(router gin.IRoutes) {
	router.POST("/auth/login", h.Login)
	router.POST("/auth/logout", h.Logout)
	router.POST("/auth/refresh-token", h.RefreshToken)
	router.GET("/auth/me", h.CurrentUser)
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), 422))
		return
	}

	result, err := h.service.Login(c.Request.Context(), req, service.LoginHistoryRecord{
		IPAddress:  c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
		DeviceName: req.DeviceName,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "login successful", result)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	token := bearerToken(c.GetHeader("Authorization"))
	if token == "" {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authorization bearer token is required", 401))
		return
	}

	if err := h.service.Logout(c.Request.Context(), token, service.LoginHistoryRecord{
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "logout successful", nil)
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	var req dto.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), 422))
		return
	}

	result, err := h.service.RefreshToken(c.Request.Context(), req, service.LoginHistoryRecord{
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "token refreshed successfully", result)
}

func (h *AuthHandler) CurrentUser(c *gin.Context) {
	token := bearerToken(c.GetHeader("Authorization"))
	if token == "" {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authorization bearer token is required", 401))
		return
	}

	result, err := h.service.CurrentUser(c.Request.Context(), token)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "current user retrieved", result)
}

func bearerToken(header string) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}
