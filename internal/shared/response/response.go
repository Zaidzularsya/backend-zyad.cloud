package response

import "github.com/gin-gonic/gin"

type Envelope struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
	Meta    any    `json:"meta,omitempty"`
}

type ErrorEnvelope struct {
	Success bool   `json:"success"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

func JSON(c *gin.Context, status int, message string, data any, meta any) {
	c.JSON(status, Envelope{
		Success: status < 400,
		Message: message,
		Data:    data,
		Meta:    meta,
	})
}

func Error(c *gin.Context, status int, code, message string) {
	c.JSON(status, ErrorEnvelope{
		Success: false,
		Code:    code,
		Message: message,
	})
}
