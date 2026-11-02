package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/service"
)

type fakePreviewService struct {
	calls   int
	filter  service.CandidateFilter
	page    int
	perPage int
	err     error
}

func (f *fakePreviewService) Summary(context.Context, coretenant.Scope) (service.CandidateSummary, error) {
	f.calls++
	return service.CandidateSummary{Target: 2, PairedAccounts: 5, Safe: 3, GraceDays: 1, Cutoff: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}, f.err
}

func (f *fakePreviewService) List(_ context.Context, _ coretenant.Scope, flt service.CandidateFilter, page, perPage int) ([]service.CandidateRow, int64, error) {
	f.calls++
	f.filter, f.page, f.perPage = flt, page, perPage
	return []service.CandidateRow{{AccountID: testAccountID, AccountName: "Budi", Status: service.CandidateWillIsolate, Balance: "100.00"}}, 51, f.err
}

func (f *fakePreviewService) ExportCSV(_ context.Context, _ coretenant.Scope, flt service.CandidateFilter, w io.Writer, begin func(int64)) error {
	f.calls++
	f.filter = flt
	if f.err != nil {
		return f.err
	}
	begin(1)
	_, err := io.WriteString(w, "Akun\nBudi\n")
	return err
}

const previewBase = "/app/mikrotik/control/preview"

var previewRoutes = []struct{ method, path string }{
	{http.MethodGet, previewBase + "/summary"},
	{http.MethodGet, previewBase},
	{http.MethodGet, previewBase + "/export"},
}

func previewRouter(t *testing.T, svc PreviewService, now func() time.Time) *gin.Engine {
	r := newTestRouter(t)
	h := NewPreviewHandler(svc)
	h.now = now
	h.RegisterRoutes(r.Group("/app/mikrotik"), nil, nil)
	return r
}

func TestPreviewRoutes_ReadPermission(t *testing.T) {
	for _, rt := range previewRoutes {
		checker := &recordingChecker{}
		svc := &fakePreviewService{}
		r := newTestRouter(t)
		r.Use(func(c *gin.Context) { permissionmiddleware.SetUserID(c, "user-1") })
		NewPreviewHandler(svc).RegisterRoutes(r.Group("/app/mikrotik"), checker, nil)
		if n := len(r.Routes()); n != len(previewRoutes) {
			t.Fatalf("registered %d routes, want %d", n, len(previewRoutes))
		}
		w := performJSON(r, rt.method, rt.path, ``)
		if w.Code != http.StatusForbidden || svc.calls != 0 {
			t.Errorf("%s: status = %d calls = %d, want 403", rt.path, w.Code, svc.calls)
		}
		if len(checker.asked) == 0 || checker.asked[0] != "mikrotik_control.read" {
			t.Errorf("%s: asked %v", rt.path, checker.asked)
		}
	}
}

func TestPreviewRoutes_Responses(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 11, 2, 18, 0, 0, 0, time.UTC) } // 3 Nov WIB
	svc := &fakePreviewService{}
	r := previewRouter(t, svc, now)

	w := performJSON(r, http.MethodGet, previewBase+"/summary", ``)
	var sum struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &sum); err != nil || w.Code != http.StatusOK ||
		sum.Data["target"] != float64(2) || sum.Data["safe"] != float64(3) || sum.Data["cutoff"] != "2026-11-01" {
		t.Fatalf("summary: %d %s", w.Code, w.Body)
	}

	w = performJSON(r, http.MethodGet, previewBase+"?status=will_isolate&q=bud&page=2&per_page=25", ``)
	var list struct {
		Data []map[string]any `json:"data"`
		Meta map[string]any   `json:"meta"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil || w.Code != http.StatusOK || len(list.Data) != 1 ||
		list.Data[0]["status"] != "will_isolate" || list.Meta["total"] != float64(51) || list.Meta["total_pages"] != float64(3) {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	if svc.filter.Status != "will_isolate" || svc.filter.Q != "bud" || svc.page != 2 || svc.perPage != 25 {
		t.Fatalf("list args: %+v %d %d", svc.filter, svc.page, svc.perPage)
	}

	svc.calls = 0
	for _, q := range []string{"?status=bogus", "?page=x", "?q=" + strings.Repeat("a", 101)} {
		if w := performJSON(r, http.MethodGet, previewBase+q, ``); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status = %d", q, w.Code)
		}
		if w := performJSON(r, http.MethodGet, previewBase+"/export"+q, ``); w.Code != http.StatusUnprocessableEntity {
			t.Errorf("export %s: status = %d", q, w.Code)
		}
	}
	if svc.calls != 0 {
		t.Fatalf("invalid queries reached the service %d times", svc.calls)
	}

	w = performJSON(r, http.MethodGet, previewBase+"/export?status=exempt", ``)
	if w.Code != http.StatusOK || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv; charset=utf-8") ||
		w.Header().Get("Content-Disposition") != `attachment; filename="isolir-kandidat-20261103.csv"` ||
		w.Body.String() != "Akun\nBudi\n" || svc.filter.Status != "exempt" {
		t.Fatalf("export: %d %v %q", w.Code, w.Header(), w.Body)
	}
}

func TestPreviewRoutes_ExportErrorIsJSON(t *testing.T) {
	svc := &fakePreviewService{err: context.DeadlineExceeded}
	w := performJSON(previewRouter(t, svc, time.Now), http.MethodGet, previewBase+"/export", ``)
	if w.Code != http.StatusInternalServerError || strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("export error: %d %v %s", w.Code, w.Header(), w.Body)
	}
}
