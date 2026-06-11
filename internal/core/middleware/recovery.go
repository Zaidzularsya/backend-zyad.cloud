package middleware

import (
	"log/slog"
	"net/http"

	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		if logger != nil {
			logger.Error("panic recovered", "panic", recovered, "request_id", RequestIDFromContext(c))
		}
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		c.Abort()
	})
}
