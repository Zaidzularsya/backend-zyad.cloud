package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
)

func failSocial(c *gin.Context, op string, err error) {
	var appErr *coreerrors.AppError
	if errors.As(err, &appErr) {
		corehttp.Fail(c, appErr)
		return
	}
	slog.ErrorContext(c.Request.Context(), "social request failed", "op", op, "error", err)
	corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", "internal server error", http.StatusInternalServerError))
}

func requireUser(c *gin.Context) (middleware.AuthenticatedUser, bool) {
	user, ok := middleware.AuthenticatedUserFromContext(c)
	if !ok || user.ID == "" {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "authenticated user is required", http.StatusUnauthorized))
		return middleware.AuthenticatedUser{}, false
	}
	return user, true
}
