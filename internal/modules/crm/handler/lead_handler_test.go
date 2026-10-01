package handler

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/crm/service"
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

// allowOnlyChecker mengizinkan permission yang terdaftar saja.
type allowOnlyChecker map[string]bool

func (a allowOnlyChecker) Can(_ context.Context, _ string, perms []string) error {
	for _, p := range perms {
		if !a[p] {
			return errors.New("denied")
		}
	}
	return nil
}
func (a allowOnlyChecker) CanOrganization(ctx context.Context, u, _ string, perms []string) error {
	return a.Can(ctx, u, perms)
}

func TestConvertWithDealRequiresDealCreate(t *testing.T) {
	r := newTestRouter(t)
	r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "u1"); c.Next() })
	NewLeadHandler(nil).RegisterRoutes(r.Group(""), allowOnlyChecker{"lead.convert": true})

	w := performJSON(r, http.MethodPost, "/leads/l1/convert",
		`{"deal":{"pipeline_id":"p1","stage_id":"s1","title":"X"}}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("convert+deal without deal.create → %d, want 403", w.Code)
	}
	w = performJSON(r, http.MethodPost, "/leads/l1/convert", `{"company":{"mode":"new","name":"PT A"}}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("convert+new company without company.create → %d, want 403", w.Code)
	}
}

func TestCreateDealForLeadRouteRequiresPermission(t *testing.T) {
	r := newTestRouter(t)
	r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "u1"); c.Next() })
	NewLeadHandler(nil).RegisterRoutes(r.Group(""), allowOnlyChecker{"lead.convert": true})

	w := performJSON(r, http.MethodPost, "/leads/l1/deal", `{"pipeline_id":"p1","stage_id":"s1","title":"X"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("POST /leads/:id/deal without deal.create → %d, want 403", w.Code)
	}
}

func TestMapLeadErrorConvertCodes(t *testing.T) {
	cases := map[error]string{
		service.ErrInvalidPipelineStage: "INVALID_PIPELINE_STAGE",
		service.ErrInvalidStartStage:    "INVALID_START_STAGE",
		service.ErrLeadNotConverted:     "LEAD_NOT_CONVERTED",
		service.ErrLeadDealExists:       "LEAD_DEAL_EXISTS",
	}
	for err, code := range cases {
		var appErr *coreerrors.AppError
		if !errors.As(mapLeadError(err), &appErr) || appErr.Code != code {
			t.Errorf("mapLeadError(%v) = %v, want code %s", err, mapLeadError(err), code)
		}
	}
}
