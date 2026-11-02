package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"

	"zyad.cloud/internal/shared/response"

	"github.com/gin-gonic/gin"
)

func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(abortFilterWriter{out: gin.DefaultErrorWriter}, func(c *gin.Context, recovered any) {
		if recovered == http.ErrAbortHandler {
			// A handler deliberately aborted a response that already started
			// (e.g. a streamed export failed midway). Re-panic so net/http
			// drops the connection; writing an error body would append it to
			// the partial response and make it look complete.
			panic(http.ErrAbortHandler)
		}
		if logger != nil {
			logger.Error("panic recovered", "panic", recovered, "request_id", RequestIDFromContext(c))
		}
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		c.Abort()
	})
}

// abortFilterWriter drops gin's stack-trace log for http.ErrAbortHandler (an
// intentional abort, which net/http itself does not log) and forwards every
// other recovery log line unchanged.
type abortFilterWriter struct{ out io.Writer }

var abortHandlerText = []byte(http.ErrAbortHandler.Error())

func (w abortFilterWriter) Write(p []byte) (int, error) {
	if w.out == nil || bytes.Contains(p, abortHandlerText) {
		return len(p), nil
	}
	return w.out.Write(p)
}
