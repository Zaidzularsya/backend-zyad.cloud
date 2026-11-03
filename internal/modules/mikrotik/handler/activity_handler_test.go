package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/service"
)

type fakeActivityService struct{ limit, offset int }

func (f *fakeActivityService) List(_ context.Context, _ coretenant.Scope, limit, offset int) ([]service.ActivityItem, int64, error) {
	f.limit, f.offset = limit, offset
	return []service.ActivityItem{{ID: "1", Event: "pilot.added", Message: "ok"}}, 7, nil
}

func TestActivityRoute_PagingAndBadQuery(t *testing.T) {
	for _, tc := range []struct {
		query         string
		code          int
		limit, offset int
	}{
		{"", 200, 50, 0},
		{"?limit=500&offset=-4", 200, 100, 0},
		{"?limit=20&offset=40", 200, 20, 40},
		{"?limit=abc", 422, 0, 0},
	} {
		r := newTestRouter(t)
		svc := &fakeActivityService{}
		h := NewActivityHandler(svc)
		r.GET("/control/activity", h.List)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/control/activity"+tc.query, nil))
		if w.Code != tc.code || svc.limit != tc.limit || svc.offset != tc.offset {
			t.Errorf("%q: code %d limit %d offset %d body %s", tc.query, w.Code, svc.limit, svc.offset, w.Body)
		}
	}
}
