package middleware

import (
	"context"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	coretenant "zyad.cloud/internal/core/tenant"

	"github.com/gin-gonic/gin"
)

const UserIDContextKey = "user_id"

type PermissionChecker interface {
	Can(ctx context.Context, userID string, requiredPermissions []string) error
}

type OrganizationPermissionChecker interface {
	CanOrganization(
		ctx context.Context,
		userID string,
		organizationID string,
		requiredPermissions []string,
	) error
}

type CombinedPermissionChecker interface {
	PermissionChecker
	OrganizationPermissionChecker
}

func Require(checker PermissionChecker, requiredPermissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if checker == nil || len(requiredPermissions) == 0 {
			c.Next()
			return
		}

		userID := UserID(c)
		if userID == "" {
			corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "user context is required", http.StatusUnauthorized))
			c.Abort()
			return
		}

		if err := checker.Can(c.Request.Context(), userID, requiredPermissions); err != nil {
			corehttp.Fail(c, err)
			c.Abort()
			return
		}

		c.Next()
	}
}

func RequireOrganization(
	checker OrganizationPermissionChecker,
	requiredPermissions ...string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if checker == nil || len(requiredPermissions) == 0 {
			c.Next()
			return
		}
		userID := UserID(c)
		if userID == "" {
			corehttp.Fail(c, coreerrors.New(
				"UNAUTHORIZED",
				"user context is required",
				http.StatusUnauthorized,
			))
			c.Abort()
			return
		}
		tenantContext, ok := coretenant.FromContext(c.Request.Context())
		if !ok {
			corehttp.Fail(c, coreerrors.New(
				"TENANT_CONTEXT_REQUIRED",
				"organization context is required",
				http.StatusForbidden,
			))
			c.Abort()
			return
		}
		if err := checker.CanOrganization(
			c.Request.Context(),
			userID,
			tenantContext.OrganizationID(),
			requiredPermissions,
		); err != nil {
			corehttp.Fail(c, err)
			c.Abort()
			return
		}
		c.Next()
	}
}

func RequireOrganizationOrGlobal(
	checker CombinedPermissionChecker,
	requiredPermissions ...string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if checker == nil || len(requiredPermissions) == 0 {
			c.Next()
			return
		}

		userID := UserID(c)
		if userID == "" {
			corehttp.Fail(c, coreerrors.New(
				"UNAUTHORIZED",
				"user context is required",
				http.StatusUnauthorized,
			))
			c.Abort()
			return
		}

		tenantContext, ok := coretenant.FromContext(c.Request.Context())
		if !ok {
			corehttp.Fail(c, coreerrors.New(
				"TENANT_CONTEXT_REQUIRED",
				"organization context is required",
				http.StatusForbidden,
			))
			c.Abort()
			return
		}

		if err := checker.CanOrganization(
			c.Request.Context(),
			userID,
			tenantContext.OrganizationID(),
			requiredPermissions,
		); err == nil {
			c.Next()
			return
		}

		if err := checker.Can(c.Request.Context(), userID, requiredPermissions); err == nil {
			c.Next()
			return
		}

		corehttp.Fail(c, coreerrors.New(
			"FORBIDDEN",
			"insufficient permissions",
			http.StatusForbidden,
		))
		c.Abort()
	}
}

func SetUserID(c *gin.Context, userID string) {
	c.Set(UserIDContextKey, userID)
}

func UserID(c *gin.Context) string {
	value, ok := c.Get(UserIDContextKey)
	if !ok {
		return ""
	}
	userID, _ := value.(string)
	return userID
}
