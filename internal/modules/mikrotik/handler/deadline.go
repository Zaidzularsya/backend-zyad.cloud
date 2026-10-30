package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Per-request write deadlines. The server-wide WriteTimeout
// (HTTP_WRITE_TIMEOUT_SECONDS, default 15s) is shorter than real router work:
// Apply/Cleanup open one connection and login per RouterOS call and took
// 15-25s in the lab, and a probe of a blackholed host can take the full Detect
// budget. Go does not cancel a handler when WriteTimeout passes, it drops the
// response, so the client would see EOF/502 while the operation completes.
const (
	// setupWriteDeadline covers Apply and Cleanup (the service bounds the
	// write phase itself to 3 minutes).
	setupWriteDeadline = 4 * time.Minute
	// previewWriteDeadline covers the read-only plan computation.
	previewWriteDeadline = 90 * time.Second
	// probeWriteDeadline covers create/update/check: DNS budget + Detect budget.
	probeWriteDeadline = 45 * time.Second
)

// extendWriteDeadline pushes this connection's write deadline to now+d. A
// writer that does not support deadlines (e.g. httptest recorders) is fine:
// the request continues with the server default.
func extendWriteDeadline(c *gin.Context, d time.Duration) {
	err := http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(d))
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		slog.Debug("mikrotik: could not extend write deadline", "error", err)
	}
}
