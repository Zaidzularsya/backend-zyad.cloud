package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
)

type denyAllChecker struct{}

func (denyAllChecker) Can(context.Context, string, []string) error { return errors.New("denied") }
func (denyAllChecker) CanOrganization(context.Context, string, string, []string) error {
	return errors.New("denied")
}

func TestParseLeadCreatedRange(t *testing.T) {
	from, toExcl, err := parseLeadCreatedRange("2026-09-01", "2026-09-28")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Bounds are Asia/Jakarta day starts expressed in UTC (00:00 WIB = 17:00Z
	// the day before), matching created_at stored as UTC.
	if !from.Equal(time.Date(2026, 8, 31, 17, 0, 0, 0, time.UTC)) || !toExcl.Equal(time.Date(2026, 9, 28, 17, 0, 0, 0, time.UTC)) {
		t.Errorf("range = %v..%v", from, toExcl)
	}

	if from, toExcl, err := parseLeadCreatedRange("", ""); err != nil || !from.IsZero() || !toExcl.IsZero() {
		t.Errorf("empty range = %v, %v, %v", from, toExcl, err)
	}
	// Same day is a valid one-day range.
	if _, _, err := parseLeadCreatedRange("2026-09-28", "2026-09-28"); err != nil {
		t.Errorf("single day: %v", err)
	}
	for _, tc := range [][2]string{{"2026-09-28", "2026-09-01"}, {"28/09/2026", ""}, {"", "yesterday"}} {
		if _, _, err := parseLeadCreatedRange(tc[0], tc[1]); err == nil {
			t.Errorf("parseLeadCreatedRange(%q, %q) expected error", tc[0], tc[1])
		}
	}
}

func TestLeadPlaybookRoutesRequirePermission(t *testing.T) {
	r := newTestRouter(t)
	r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "u1"); c.Next() })
	NewLeadHandler(nil).RegisterRoutes(r.Group(""), denyAllChecker{})

	cases := []struct{ method, path string }{
		{http.MethodPost, "/leads/l1/disqualify"},
		{http.MethodPost, "/leads/l1/playbook/start"},
		{http.MethodGet, "/leads/l1/events"},
	}
	for _, c := range cases {
		w := performJSON(r, c.method, c.path, `{"reason":"duplicate"}`)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s without permission → %d, want 403", c.method, c.path, w.Code)
		}
	}
}
