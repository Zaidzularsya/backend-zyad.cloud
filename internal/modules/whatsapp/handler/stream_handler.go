package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/modules/whatsapp/realtime"
	"zyad.cloud/internal/modules/whatsapp/service"
)

const (
	streamHeartbeat = 20 * time.Second
	// streamMaxLifetime bounds how long a stream can outlive the caller's
	// token and permissions (checked once, at connect); the client reconnects.
	streamMaxLifetime = 30 * time.Minute
)

// WithStream enables GET /stream, fed by the given subscriber.
func (h *ConversationHandler) WithStream(subscriber realtime.Subscriber) *ConversationHandler {
	h.stream = subscriber
	return h
}

// visibleTo applies the same rule as the REST API (service.Viewer.canSee):
// read_all sees every conversation, everyone else only their own.
func visibleTo(viewer service.Viewer, event realtime.Event) bool {
	return viewer.CanReadAll || (viewer.UserID != "" && event.AssigneeUserID == viewer.UserID)
}

// Stream is a Server-Sent Events feed of change hints for the caller's
// organization. Clients re-read state through the normal endpoints, so
// events carry ids only. Redis delivery is at-most-once: clients must
// refetch after every (re)connect.
func (h *ConversationHandler) Stream(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	viewer := h.viewer(c, scope)

	ctx := c.Request.Context()
	events, cancel, err := h.stream.Subscribe(ctx, scope.OrganizationID())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("REALTIME_UNAVAILABLE", "realtime stream unavailable", http.StatusServiceUnavailable))
		return
	}
	defer cancel()

	header := c.Writer.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no") // nginx: do not buffer the stream

	// The server-wide WriteTimeout would cut a long-lived stream.
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetWriteDeadline(time.Time{})

	c.Status(http.StatusOK)
	if _, err := fmt.Fprint(c.Writer, ": connected\n\n"); err != nil {
		return
	}
	_ = controller.Flush()

	heartbeat := time.NewTicker(streamHeartbeat)
	defer heartbeat.Stop()
	lifetime := time.NewTimer(streamMaxLifetime)
	defer lifetime.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-lifetime.C:
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return
			}
		case event, open := <-events:
			if !open {
				return
			}
			if !visibleTo(viewer, event) {
				continue
			}
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(c.Writer, "event: wa\ndata: %s\n\n", payload); err != nil {
				return
			}
		}
		_ = controller.Flush()
	}
}
