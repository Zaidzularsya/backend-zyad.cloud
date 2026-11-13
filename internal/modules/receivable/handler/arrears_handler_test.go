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

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/service"
)

const (
	arrCtrID = "44444444-4444-4444-4444-444444444444"
	recID = "55555555-5555-5555-5555-555555555555"
)

type fakeArrearsSvc struct {
	recorded  service.ArrearsInput
	calls     int
	recordErr error
	flagValue *bool
}

func (f *fakeArrearsSvc) Preview(context.Context, coretenant.Scope, string) ([]service.ArrearsPreviewRow, bool, error) {
	start, end := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	return []service.ArrearsPreviewRow{{Period: service.ArrearsPeriod{Index: 1, Start: start, End: end, DefaultAmount: "333000.00"}, Conflict: "x"}}, true, nil
}
func (f *fakeArrearsSvc) Record(_ context.Context, _ coretenant.Scope, in service.ArrearsInput, _ string) (service.ArrearsResult, error) {
	f.calls++
	f.recorded = in
	if f.recordErr != nil {
		return service.ArrearsResult{}, f.recordErr
	}
	return service.ArrearsResult{
		Recording: domain.ArrearsRecording{ID: recID, NotifyStatus: domain.ArrearsNotifyFailed, NotifyError: "Email: x"},
		Invoices:  []domain.Invoice{{ID: invID, Status: domain.InvoiceOverdue, GrandTotal: "1.00", AmountPaid: "0.00", ArrearsRecordingID: recID}},
	}, nil
}
func (f *fakeArrearsSvc) Resend(context.Context, coretenant.Scope, string, string) (domain.ArrearsRecording, error) {
	return domain.ArrearsRecording{ID: recID, NotifyStatus: domain.ArrearsNotifySent}, nil
}
func (f *fakeArrearsSvc) SetIsolirFlag(_ context.Context, _ coretenant.Scope, _ string, value bool, _ string) (domain.Invoice, error) {
	f.flagValue = &value
	return domain.Invoice{ID: invID, Status: domain.InvoiceOverdue, IsolirFlag: value, GrandTotal: "1.00", AmountPaid: "0.00"}, nil
}

func arrearsRouter(t *testing.T, svc *fakeArrearsSvc, perms ...string) *gin.Engine {
	t.Helper()
	f := newFixture(t, perms...) // also registers the invoice routes, to prove there is no route clash
	NewArrearsHandler(svc).RegisterRoutes(f.r.Group("/app/receivable"), grant(perms...))
	return f.r
}

func callWithKey(r http.Handler, method, path, body, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const arrearsBody = `{"account_id":"` + accID + `","contract_id":"` + arrCtrID + `","reason":"lama",
	"rows":[{"period_start":"2026-08-05","amount":"333000","due_date":"2026-08-12","isolir_flag":false}]}`

func TestArrearsRecord_MapsRequestAndReturnsResult(t *testing.T) {
	svc := &fakeArrearsSvc{}
	r := arrearsRouter(t, svc, "invoice.record_arrears", "invoice.read")
	w := callWithKey(r, http.MethodPost, base("/invoices/arrears"), arrearsBody, "dialog-1")
	if w.Code != http.StatusCreated {
		t.Fatalf("code=%d body=%s", w.Code, w.Body.String())
	}
	in := svc.recorded
	if in.IdempotencyKey != "dialog-1" || in.AccountID != accID || in.ContractID != arrCtrID || in.Reason != "lama" || len(in.Rows) != 1 ||
		in.Rows[0].PeriodStart.Format("2006-01-02") != "2026-08-05" || in.Rows[0].DueDate.Format("2006-01-02") != "2026-08-12" ||
		in.Rows[0].Amount != "333000" {
		t.Fatalf("input = %+v", in)
	}
	var out struct {
		Data struct {
			Recording    map[string]any   `json:"recording"`
			Invoices     []map[string]any `json:"invoices"`
			NotifyStatus string           `json:"notify_status"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out.Data.NotifyStatus != "failed" || len(out.Data.Invoices) != 1 || out.Data.Recording["id"] != recID ||
		out.Data.Invoices[0]["arrears_recording_id"] != recID || out.Data.Invoices[0]["isolir_flag"] != false {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestArrearsRecord_RequiresIdempotencyKey(t *testing.T) {
	svc := &fakeArrearsSvc{}
	r := arrearsRouter(t, svc, "invoice.record_arrears")
	for _, key := range []string{"", strings.Repeat("k", 101)} {
		if w := callWithKey(r, http.MethodPost, base("/invoices/arrears"), arrearsBody, key); w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("key len %d: code=%d", len(key), w.Code)
		}
	}
	if svc.calls != 0 {
		t.Fatal("service must not be called")
	}
}

func TestArrearsRecord_InvalidBody(t *testing.T) {
	svc := &fakeArrearsSvc{}
	r := arrearsRouter(t, svc, "invoice.record_arrears")
	for _, body := range []string{
		`{"account_id":"x","contract_id":"` + arrCtrID + `","reason":"a","rows":[]}`,
		`{"account_id":"` + accID + `","contract_id":"` + arrCtrID + `","reason":"a","rows":[{"period_start":"5-8-2026","amount":"1","due_date":"2026-08-12"}]}`,
		`not json`,
	} {
		if w := callWithKey(r, http.MethodPost, base("/invoices/arrears"), body, "k"); w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("body %s: code=%d", body, w.Code)
		}
	}
}

func TestArrearsRoutes_Permissions(t *testing.T) {
	svc := &fakeArrearsSvc{}
	none := arrearsRouter(t, svc, "invoice.read", "invoice.create")
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/contracts/" + arrCtrID + "/arrears/periods", ""},
		{http.MethodPost, "/invoices/arrears", arrearsBody},
		{http.MethodPost, "/invoices/arrears/" + recID + "/resend", ""},
		{http.MethodPut, "/invoices/" + invID + "/isolir-flag", `{"value":true}`},
	} {
		if w := callWithKey(none, tc.method, base(tc.path), tc.body, "k"); w.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", tc.method, tc.path, w.Code)
		}
	}

	admin := arrearsRouter(t, svc, "invoice.record_arrears")
	if w := callWithKey(admin, http.MethodPut, base("/invoices/"+invID+"/isolir-flag"), `{"value":true}`, ""); w.Code != http.StatusForbidden {
		t.Fatalf("admin isolir-flag = %d, want 403", w.Code)
	}
	flagged := strings.Replace(arrearsBody, `"isolir_flag":false`, `"isolir_flag":true`, 1)
	if w := callWithKey(admin, http.MethodPost, base("/invoices/arrears"), flagged, "k"); w.Code != http.StatusForbidden {
		t.Fatalf("admin record with flag = %d, want 403", w.Code)
	}
	if w := callWithKey(admin, http.MethodGet, base("/contracts/"+arrCtrID+"/arrears/periods"), "", ""); w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), `"has_trigger_product":true`) || !strings.Contains(w.Body.String(), `"default_amount":"333000.00"`) ||
		!strings.Contains(w.Body.String(), `"start":"2026-08-05"`) {
		t.Fatalf("periods = %d %s", w.Code, w.Body.String())
	}

	owner := arrearsRouter(t, svc, "invoice.record_arrears", "invoice.isolir_flag")
	if w := callWithKey(owner, http.MethodPut, base("/invoices/"+invID+"/isolir-flag"), `{"value":true}`, ""); w.Code != http.StatusOK ||
		svc.flagValue == nil || !*svc.flagValue {
		t.Fatalf("owner isolir-flag = %d %s", w.Code, w.Body.String())
	}
	if w := callWithKey(owner, http.MethodPost, base("/invoices/arrears"), flagged, "k2"); w.Code != http.StatusCreated {
		t.Fatalf("owner record with flag = %d", w.Code)
	}
	if w := callWithKey(owner, http.MethodPost, base("/invoices/arrears/"+recID+"/resend"), "", ""); w.Code != http.StatusOK {
		t.Fatalf("resend = %d", w.Code)
	}
}

func TestArrearsRecord_ErrorMapping(t *testing.T) {
	cases := []struct {
		err  error
		code int
		want string
	}{
		{&service.ArrearsConflictError{Conflicts: []service.ArrearsConflict{{Row: 2, PeriodStart: "2026-09-05", Reason: "Periode ini sudah punya invoice."}}},
			http.StatusConflict, `"conflicts":[{"row":2,"period_start":"2026-09-05","reason":"Periode ini sudah punya invoice."}]`},
		{service.ErrArrearsContractMissing, http.StatusUnprocessableEntity, "onboarding"},
		{service.ErrArrearsInvalid, http.StatusUnprocessableEntity, "VALIDATION_ERROR"},
	}
	for _, tc := range cases {
		svc := &fakeArrearsSvc{recordErr: tc.err}
		r := arrearsRouter(t, svc, "invoice.record_arrears")
		w := callWithKey(r, http.MethodPost, base("/invoices/arrears"), arrearsBody, "k")
		if w.Code != tc.code || !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("%v: code=%d body=%s", tc.err, w.Code, w.Body.String())
		}
	}
}

func TestArrearsRoutes_DoNotShadowInvoiceRoutes(t *testing.T) {
	svc := &fakeArrearsSvc{}
	r := arrearsRouter(t, svc, "invoice.read", "invoice.record_arrears")
	if w := call(r, http.MethodGet, base("/invoices/"+invID), ""); w.Code != http.StatusOK {
		t.Fatalf("GET /invoices/:id = %d", w.Code)
	}
	if svc.calls != 0 {
		t.Fatal("arrears handler hit by a normal invoice route")
	}
	_ = permissionmiddleware.UserID
}
