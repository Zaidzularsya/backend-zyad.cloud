package handler

import (
	"context"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/notification/domain"
	"zyad.cloud/internal/core/notification/dto"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"

	"github.com/gin-gonic/gin"
)

type NotificationPreferenceService interface {
	GetUserPreferences(ctx context.Context, userID string, organizationID string) ([]domain.NotificationPreference, error)
	UpdateUserPreferences(ctx context.Context, userID string, organizationID string, req dto.BulkPreferenceRequest) ([]domain.NotificationPreference, error)
}

type PreferenceHandler struct {
	service NotificationPreferenceService
	checker permissionmiddleware.PermissionChecker
}

func NewPreferenceHandler(preferenceService NotificationPreferenceService, checker permissionmiddleware.PermissionChecker) *PreferenceHandler {
	return &PreferenceHandler{
		service: preferenceService,
		checker: checker,
	}
}

func (h *PreferenceHandler) RegisterRoutes(router *gin.RouterGroup) {
	users := router.Group("/users/me/notification-preferences")
	users.GET("", h.ensureService(), h.GetMyPreferences)
	users.PATCH("", h.ensureService(), h.UpdateMyPreferences)

	admin := router.Group("/admin/users/:userId/notification-preferences")
	admin.GET("", h.ensureService(), h.require("notification_preference.read"), h.GetUserPreferences)
	admin.PATCH("", h.ensureService(), h.require("notification_preference.manage"), h.UpdateUserPreferences)
}

func (h *PreferenceHandler) GetMyPreferences(c *gin.Context) {
	userID := permissionmiddleware.UserID(c)
	if userID == "" {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "user context is required", http.StatusUnauthorized))
		return
	}

	preferences, err := h.service.GetUserPreferences(c.Request.Context(), userID, c.Query("organization_id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "notification preferences retrieved", dto.NewPreferenceResponses(preferences))
}

func (h *PreferenceHandler) UpdateMyPreferences(c *gin.Context) {
	userID := permissionmiddleware.UserID(c)
	if userID == "" {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "user context is required", http.StatusUnauthorized))
		return
	}

	var req dto.BulkPreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	preferences, err := h.service.UpdateUserPreferences(c.Request.Context(), userID, c.Query("organization_id"), req)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "notification preferences updated", dto.NewPreferenceResponses(preferences))
}

func (h *PreferenceHandler) GetUserPreferences(c *gin.Context) {
	preferences, err := h.service.GetUserPreferences(c.Request.Context(), c.Param("userId"), c.Query("organization_id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "notification preferences retrieved", dto.NewPreferenceResponses(preferences))
}

func (h *PreferenceHandler) UpdateUserPreferences(c *gin.Context) {
	var req dto.BulkPreferenceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	preferences, err := h.service.UpdateUserPreferences(c.Request.Context(), c.Param("userId"), c.Query("organization_id"), req)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "notification preferences updated", dto.NewPreferenceResponses(preferences))
}

func (h *PreferenceHandler) ensureService() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h.service == nil {
			corehttp.Fail(c, coreerrors.New("NOTIFICATION_PREFERENCE_SERVICE_REQUIRED", "notification preference service is required", http.StatusInternalServerError))
			c.Abort()
			return
		}
		c.Next()
	}
}

func (h *PreferenceHandler) require(permission string, additionalPermissions ...string) gin.HandlerFunc {
	required := append([]string{permission}, additionalPermissions...)
	return permissionmiddleware.Require(h.checker, required...)
}
