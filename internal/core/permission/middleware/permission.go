package middleware

import (
	"context"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"

	"github.com/gin-gonic/gin"
)

const UserIDContextKey = "user_id"

type PermissionChecker interface {
	Can(ctx context.Context, userID string, requiredPermissions []string) error
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
