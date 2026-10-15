package handler

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/whatsapp/realtime"
	"zyad.cloud/internal/modules/whatsapp/service"
)

type fakeSubscriber struct {
	events    chan realtime.Event
	orgID     string
	cancelled chan struct{}
}

func (f *fakeSubscriber) Subscribe(_ context.Context, organizationID string) (<-chan realtime.Event, func(), error) {
	f.orgID = organizationID
	return f.events, func() { close(f.cancelled) }, nil
}

func TestVisibleTo(t *testing.T) {
	event := realtime.Event{AssigneeUserID: "user-1"}
	cases := []struct {
		name   string
		viewer service.Viewer
		want   bool
	}{
		{"read_all sees everything", service.Viewer{UserID: "other", CanReadAll: true}, true},
		{"assignee sees own", service.Viewer{UserID: "user-1"}, true},
		{"other user does not", service.Viewer{UserID: "user-2"}, false},
		{"anonymous does not", service.Viewer{}, false},
	}
	for _, tc := range cases {
		if got := visibleTo(tc.viewer, event); got != tc.want {
			t.Errorf("%s: visibleTo = %v, want %v", tc.name, got, tc.want)
		}
	}
	if visibleTo(service.Viewer{}, realtime.Event{}) {
		t.Error("viewer without id must not see an unassigned conversation")
	}
}

func TestStreamDeliversVisibleEventsOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     "org-1",
		OrganizationSlug:   "acme",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		ResolutionSource:   coretenant.ResolutionSourceWorker,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext: %v", err)
	}

	sub := &fakeSubscriber{events: make(chan realtime.Event, 4), cancelled: make(chan struct{})}
	h := NewConversationHandler(nil).WithStream(sub)

	router := gin.New()
	router.GET("/stream", func(c *gin.Context) {
		c.Set(permissionmiddleware.UserIDContextKey, "user-1")
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tenantContext))
		c.Next()
	}, h.Stream)
	server := httptest.NewServer(router)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /stream: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := resp.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("X-Accel-Buffering = %q", got)
	}

	sub.events <- realtime.Event{Type: realtime.EventMessage, ConversationID: "hidden", AssigneeUserID: "user-2"}
	sub.events <- realtime.Event{Type: realtime.EventMessage, ConversationID: "mine", AssigneeUserID: "user-1"}

	lines := make(chan string, 16)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	var data string
	deadline := time.After(3 * time.Second)
	for data == "" {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatal("stream closed before an event arrived")
			}
			if strings.HasPrefix(line, "data: ") {
				data = line
			}
		case <-deadline:
			t.Fatal("timed out waiting for event")
		}
	}
	if !strings.Contains(data, `"conversation_id":"mine"`) {
		t.Errorf("first delivered event = %s, want the visible one only", data)
	}
	if sub.orgID != "org-1" {
		t.Errorf("subscribed org = %q, want org-1", sub.orgID)
	}

	cancel()
	select {
	case <-sub.cancelled:
	case <-time.After(3 * time.Second):
		t.Fatal("subscription not released after client disconnect")
	}
}
