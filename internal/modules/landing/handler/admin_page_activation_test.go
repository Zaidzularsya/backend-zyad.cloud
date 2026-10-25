package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type fakeActivationReader struct {
	open     map[string]domain.LandingPageActivation
	received []string
}

func (f *fakeActivationReader) OpenActivationsByPages(_ context.Context, _ coretenant.Scope, ids []string) (map[string]domain.LandingPageActivation, error) {
	f.received = append(f.received, ids...)
	return f.open, nil
}

func TestAdminPageListIncludesActivationFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	past := now.Add(-time.Hour)
	future := now.Add(48 * time.Hour)
	due := now.Add(time.Hour)

	pageSvc := &fakePageService{listPages: []domain.LandingPage{
		{ID: "p-preview", Slug: "a", Status: domain.PageStatusDraft},
		{ID: "p-publish", Slug: "b", Status: domain.PageStatusPublished, ActiveUntil: &future},
		{ID: "p-expired", Slug: "c", Status: domain.PageStatusPublished, ActiveUntil: &past},
		{ID: "p-none", Slug: "d", Status: domain.PageStatusPublished},
	}}
	reader := &fakeActivationReader{open: map[string]domain.LandingPageActivation{
		"p-preview": {
			ID: "act-1", PageID: "p-preview", BuyerName: "Budi", Price: "150000.00",
			Status: domain.ActivationStatusOpen, InvoiceDueAt: due, PreviewTokenHash: "SECRET-HASH",
		},
	}}
	h := NewAdminPageHandler(pageSvc, nil, nil, nil).WithActivations(reader)

	router := gin.New()
	router.GET("/admin/landing-pages", func(c *gin.Context) {
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), verifiedTenantContext(t)))
		h.ListPages(c)
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/landing-pages", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "SECRET-HASH") {
		t.Fatal("admin response must not expose the preview token hash")
	}

	var body struct {
		Data struct {
			Items []struct {
				ID              string     `json:"id"`
				ActivationState string     `json:"activation_state"`
				ActiveUntil     *time.Time `json:"active_until"`
				OpenActivation  *struct {
					ID                 string    `json:"id"`
					BuyerName          string    `json:"buyer_name"`
					Price              string    `json:"price"`
					InvoiceDueAt       time.Time `json:"invoice_due_at"`
					PreviewURLTemplate string    `json:"preview_url_template"`
				} `json:"open_activation"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := map[string]string{"p-preview": "preview", "p-publish": "publish", "p-expired": "expired", "p-none": "none"}
	if len(body.Data.Items) != 4 {
		t.Fatalf("items = %d", len(body.Data.Items))
	}
	for _, it := range body.Data.Items {
		if it.ActivationState != want[it.ID] {
			t.Fatalf("%s: activation_state = %q, want %q", it.ID, it.ActivationState, want[it.ID])
		}
		if it.ID == "p-preview" {
			oa := it.OpenActivation
			if oa == nil || oa.ID != "act-1" || oa.BuyerName != "Budi" || oa.Price != "150000.00" || !oa.InvoiceDueAt.Equal(due) {
				t.Fatalf("open_activation = %+v", oa)
			}
			if oa.PreviewURLTemplate != "/preview/{token}" {
				t.Fatalf("preview_url_template = %q", oa.PreviewURLTemplate)
			}
		} else if it.OpenActivation != nil {
			t.Fatalf("%s: unexpected open_activation", it.ID)
		}
		if it.ID == "p-publish" && (it.ActiveUntil == nil || !it.ActiveUntil.Equal(future)) {
			t.Fatalf("active_until = %v", it.ActiveUntil)
		}
	}
	if len(reader.received) != 4 {
		t.Fatalf("activations must be fetched once for all listed pages, got %v", reader.received)
	}
}

func TestAdminPageGetWithoutActivationReaderKeepsNoneState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	pageSvc := &fakePageService{getPage: domain.LandingPage{ID: "p1", Slug: "x", Status: domain.PageStatusPublished}}
	h := NewAdminPageHandler(pageSvc, nil, nil, nil)

	router := gin.New()
	router.GET("/admin/landing-pages/:id", func(c *gin.Context) {
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), verifiedTenantContext(t)))
		h.GetPage(c)
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/landing-pages/p1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"activation_state":"none"`) ||
		!strings.Contains(rec.Body.String(), `"active_until":null`) ||
		!strings.Contains(rec.Body.String(), `"open_activation":null`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
