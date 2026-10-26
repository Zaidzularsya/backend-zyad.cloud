package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/service"
)

type fakeActivationManager struct {
	list      service.ActivationList
	query     service.ActivationListQuery
	renewID   string
	renewIn   service.ActivationRenewInput
	renewUser string
	orgID     string
	err       error
	calls     int
}

func (f *fakeActivationManager) List(_ context.Context, scope coretenant.Scope, q service.ActivationListQuery) (service.ActivationList, error) {
	f.calls++
	f.query, f.orgID = q, scope.OrganizationID()
	return f.list, f.err
}

func (f *fakeActivationManager) Renew(_ context.Context, scope coretenant.Scope, id string, in service.ActivationRenewInput, userID string) (service.ActivationRenewResult, error) {
	f.calls++
	f.renewID, f.renewIn, f.renewUser, f.orgID = id, in, userID, scope.OrganizationID()
	if f.err != nil {
		return service.ActivationRenewResult{}, f.err
	}
	invoice := "inv-1"
	return service.ActivationRenewResult{
		Activation: domain.LandingPageActivation{
			ID: id, PageID: "page-a", InvoiceID: &invoice, BuyerName: "Budi", BuyerContact: "0812", Price: "150000.00",
			PreviewTokenHash: "SECRET-HASH", InvoiceDueAt: time.Date(2026, 10, 24, 10, 0, 0, 0, time.UTC),
			Status: domain.ActivationStatusOpen,
		},
		PreviewURL: "https://r.example.test/preview/NEWTOKEN",
		PaymentURL: "https://app.example.test/i/PAY",
	}, nil
}

type invoicesChecker struct{ allow bool }

func (c invoicesChecker) Can(context.Context, string, []string) error { return c.err() }
func (c invoicesChecker) CanOrganization(context.Context, string, string, []string) error {
	return c.err()
}
func (c invoicesChecker) err() error {
	if c.allow {
		return nil
	}
	return coreerrors.New("FORBIDDEN", "permission denied", http.StatusForbidden)
}

func serveInvoices(t *testing.T, mgr service.ActivationManager, allow bool, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), verifiedTenantContext(t)))
		c.Set(permissionmiddleware.UserIDContextKey, "user-1")
		c.Next()
	})
	NewActivationInvoicesHandler(mgr).RegisterRoutes(&router.RouterGroup, invoicesChecker{allow: allow})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

func sampleList() service.ActivationList {
	invoice := "inv-1"
	return service.ActivationList{
		Total: 31, Page: 2, PerPage: 20,
		Items: []service.ActivationListEntry{{
			Activation: domain.LandingPageActivation{
				ID: "act-1", PageID: "page-a", InvoiceID: &invoice, BuyerName: "Budi", BuyerContact: "0812", Price: "150000.00",
				DurationValue: 1, DurationUnit: domain.ActivationDurationMonth, PreviewTokenHash: "SECRET-HASH",
				InvoiceDueAt: time.Date(2026, 10, 23, 11, 0, 0, 0, time.UTC), Status: domain.ActivationStatusOpen,
			},
			PageTitle: "Undangan Budi", PageSlug: "undangan-budi", PageStatus: domain.PageStatusDraft,
			State: service.ActivationEntryAwaitingPayment, PaymentURL: "https://app.example.test/i/PAY",
		}},
	}
}

func TestActivationInvoicesListReturnsItemsAndMeta(t *testing.T) {
	mgr := &fakeActivationManager{list: sampleList()}
	rec := serveInvoices(t, mgr, true, http.MethodGet, "/app/landing/activations?status=open&page=2&per_page=20&org=other", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	if mgr.orgID != "22222222-2222-2222-2222-222222222222" || mgr.query.Status != "open" || mgr.query.Page != 2 || mgr.query.PerPage != 20 {
		t.Fatalf("org=%s query=%+v", mgr.orgID, mgr.query)
	}
	body := rec.Body.String()
	if strings.Contains(body, "SECRET-HASH") {
		t.Fatal("response must not expose the token hash")
	}
	var resp struct {
		Data struct {
			Items []struct {
				ID         string `json:"id"`
				State      string `json:"state"`
				PaymentURL string `json:"payment_url"`
				BuyerName  string `json:"buyer_name"`
				PageTitle  string `json:"page_title"`
				PageSlug   string `json:"page_slug"`
				Price      string `json:"price"`
				PageID     string `json:"page_id"`
			} `json:"items"`
			Meta struct {
				Total      int64 `json:"total"`
				Page       int   `json:"page"`
				PerPage    int   `json:"per_page"`
				TotalPages int   `json:"total_pages"`
			} `json:"meta"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	it := resp.Data.Items[0]
	if len(resp.Data.Items) != 1 || it.State != "awaiting_payment" || it.PaymentURL != "https://app.example.test/i/PAY" ||
		it.PageTitle != "Undangan Budi" || it.Price != "150000.00" || it.PageID != "page-a" {
		t.Fatalf("items = %+v", resp.Data.Items)
	}
	if resp.Data.Meta.Total != 31 || resp.Data.Meta.TotalPages != 2 {
		t.Fatalf("meta = %+v", resp.Data.Meta)
	}
}

func TestActivationInvoicesRenewReturnsNewLinksOnce(t *testing.T) {
	mgr := &fakeActivationManager{}
	rec := serveInvoices(t, mgr, true, http.MethodPost, "/app/landing/activations/act-1/renew", `{"invoice_due_days":2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}
	if mgr.renewID != "act-1" || mgr.renewUser != "user-1" || mgr.renewIn.InvoiceDueDays == nil || *mgr.renewIn.InvoiceDueDays != 2 || mgr.renewIn.InvoiceDueHours != nil {
		t.Fatalf("renew id=%s user=%s in=%+v", mgr.renewID, mgr.renewUser, mgr.renewIn)
	}
	body := rec.Body.String()
	if strings.Contains(body, "SECRET-HASH") {
		t.Fatal("response must not expose the token hash")
	}
	if !strings.Contains(body, `"preview_url":"https://r.example.test/preview/NEWTOKEN"`) ||
		!strings.Contains(body, `"payment_url":"https://app.example.test/i/PAY"`) {
		t.Fatalf("response = %s", body)
	}
}

func TestActivationInvoicesRenewAcceptsEmptyBody(t *testing.T) {
	mgr := &fakeActivationManager{}
	rec := serveInvoices(t, mgr, true, http.MethodPost, "/app/landing/activations/act-1/renew", ``)
	if rec.Code != http.StatusOK || mgr.renewIn.InvoiceDueDays != nil || mgr.renewIn.InvoiceDueHours != nil {
		t.Fatalf("status=%d in=%+v", rec.Code, mgr.renewIn)
	}
}

func TestActivationInvoicesRenewRejectsBadJSON(t *testing.T) {
	mgr := &fakeActivationManager{}
	rec := serveInvoices(t, mgr, true, http.MethodPost, "/app/landing/activations/act-1/renew", `{"invoice_due_days":"x"}`)
	if rec.Code != http.StatusUnprocessableEntity || mgr.calls != 0 {
		t.Fatalf("status=%d calls=%d", rec.Code, mgr.calls)
	}
}

func TestActivationInvoicesRequirePagePermission(t *testing.T) {
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/app/landing/activations"},
		{http.MethodPost, "/app/landing/activations/act-1/renew"},
	} {
		mgr := &fakeActivationManager{}
		rec := serveInvoices(t, mgr, false, tc.method, tc.path, `{}`)
		if rec.Code != http.StatusForbidden || mgr.calls != 0 {
			t.Fatalf("%s %s: status=%d calls=%d", tc.method, tc.path, rec.Code, mgr.calls)
		}
	}
}

func TestActivationInvoicesPropagateServiceErrors(t *testing.T) {
	mgr := &fakeActivationManager{err: coreerrors.New("ACTIVATION_NOT_OPEN", "x", http.StatusConflict)}
	if rec := serveInvoices(t, mgr, true, http.MethodPost, "/app/landing/activations/a/renew", `{}`); rec.Code != http.StatusConflict {
		t.Fatalf("status=%d", rec.Code)
	}
	mgr = &fakeActivationManager{err: errors.New("boom")}
	if rec := serveInvoices(t, mgr, true, http.MethodGet, "/app/landing/activations", ""); rec.Code < 500 {
		t.Fatalf("status=%d, want 5xx", rec.Code)
	}
}
