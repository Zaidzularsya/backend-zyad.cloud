package http

import (
	"errors"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

func OK(c *gin.Context, message string, data any) {
	response.JSON(c, http.StatusOK, message, data, nil)
}

func Created(c *gin.Context, message string, data any) {
	response.JSON(c, http.StatusCreated, message, data, nil)
}

func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func Fail(c *gin.Context, err error) {
	var appErr *coreerrors.AppError
	if errors.As(err, &appErr) {
		response.Error(c, appErr.Status, appErr.Code, appErr.Message)
		return
	}

	response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}
