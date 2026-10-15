package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/modules/receivable/service"
	"zyad.cloud/internal/shared/pricing"
)

const (
	invID = "22222222-2222-2222-2222-222222222222"
	accID = "33333333-3333-3333-3333-333333333333"
)

// grantChecker mengizinkan hanya permission yang terdaftar (semua yang diminta harus ada).
type grantChecker struct{ granted map[string]bool }

func (g grantChecker) check(perms []string) error {
	for _, p := range perms {
		if !g.granted[p] {
			return http.ErrNotSupported
		}
	}
	return nil
}
func (g grantChecker) Can(_ context.Context, _ string, perms []string) error { return g.check(perms) }
func (g grantChecker) CanOrganization(_ context.Context, _, _ string, perms []string) error {
	return g.check(perms)
}

func grant(perms ...string) grantChecker {
	m := map[string]bool{}
	for _, p := range perms {
		m[p] = true
	}
	return grantChecker{granted: m}
}

var allReceivablePerms = []string{"invoice.read", "invoice.create", "invoice.update", "invoice.send", "invoice.mark_paid", "invoice.void",
	"receivable.settings", "email.send", "whatsapp.message.send"}

// ---- fakes (embedding antarmuka: hanya method yang dipakai test yang diisi) ----

type fakeInvoices struct {
	service.InvoiceService
	created, updated service.InvoiceInput
	issuedBy         string
	listFilter       repository.InvoiceListFilter
	listed           []domain.Invoice
	getErr           error
	voidErr          error
	pdfErr           error
	invoice          domain.Invoice
}

func (f *fakeInvoices) Get(context.Context, coretenant.Scope, string) (domain.Invoice, error) {
	return f.invoice, f.getErr
}
func (f *fakeInvoices) CreateDraft(_ context.Context, _ coretenant.Scope, in service.InvoiceInput, _ string) (domain.Invoice, error) {
	f.created = in
	return f.invoice, nil
}
func (f *fakeInvoices) UpdateDraft(_ context.Context, _ coretenant.Scope, _ string, in service.InvoiceInput, _ string) (domain.Invoice, error) {
	f.updated = in
	return f.invoice, nil
}
func (f *fakeInvoices) Issue(_ context.Context, _ coretenant.Scope, _, actor string) (domain.Invoice, error) {
	f.issuedBy = actor
	return f.invoice, nil
}
func (f *fakeInvoices) List(_ context.Context, _ coretenant.Scope, flt repository.InvoiceListFilter) ([]domain.Invoice, int64, error) {
	f.listFilter = flt
	return f.listed, int64(len(f.listed)), nil
}
func (f *fakeInvoices) Void(_ context.Context, _ coretenant.Scope, _, _, _ string) (domain.Invoice, error) {
	return f.invoice, f.voidErr
}
func (f *fakeInvoices) PDF(context.Context, coretenant.Scope, string) (service.InvoicePDF, error) {
	return service.InvoicePDF{Filename: "INV-1.pdf", Content: []byte("%PDF")}, f.pdfErr
}
func (f *fakeInvoices) Link(context.Context, coretenant.Scope, string, string) (string, time.Time, error) {
	return "https://app.test/i/tok", time.Date(2027, 1, 10, 16, 59, 59, 0, time.UTC), nil
}

type fakeSends struct {
	service.SendService
	in       service.SendInput
	err      error
	sendCall int
	last     map[string]domain.Send
	list     []domain.Send
}

func (f *fakeSends) Send(_ context.Context, _ coretenant.Scope, id string, in service.SendInput, _ string) (domain.Send, error) {
	f.sendCall++
	f.in = in
	return domain.Send{ID: "s1", InvoiceID: id, Channel: in.Channel, Status: "sent", Trigger: "manual"}, f.err
}
func (f *fakeSends) List(context.Context, coretenant.Scope, string) ([]domain.Send, error) {
	return f.list, nil
}
func (f *fakeSends) LatestByInvoices(context.Context, coretenant.Scope, []string) (map[string]domain.Send, error) {
	return f.last, nil
}

type fakePayments struct {
	service.PaymentService
	in  service.ManualPaymentInput
	err error
}

func (f *fakePayments) RecordManual(_ context.Context, _ coretenant.Scope, id string, in service.ManualPaymentInput, _ string) (domain.Invoice, domain.Payment, error) {
	f.in = in
	if f.err != nil {
		return domain.Invoice{}, domain.Payment{}, f.err
	}
	return domain.Invoice{ID: id, Status: domain.InvoicePaid, GrandTotal: "100.00", AmountPaid: "100.00"}, domain.Payment{ID: "p1", InvoiceID: id, Amount: "100.00", Method: "manual"}, nil
}
func (f *fakePayments) List(context.Context, coretenant.Scope, string) ([]domain.Payment, error) {
	return []domain.Payment{{ID: "p1", InvoiceID: invID, Amount: "100.00", Method: "manual"}}, nil
}
func (f *fakePayments) ListAll(_ context.Context, _ coretenant.Scope, limit, offset int) ([]domain.Payment, int64, error) {
	return []domain.Payment{{ID: "p1", InvoiceID: invID, InvoiceNumber: "INV-1", AccountName: "Budi", Amount: "100.00", Method: "manual"}}, 25, nil
}

type fakeAccountSvc struct {
	service.AccountService
	created service.AccountInput
	getErr  error
}

func (f *fakeAccountSvc) Create(_ context.Context, _ coretenant.Scope, in service.AccountInput, _ string) (domain.Account, error) {
	f.created = in
	return domain.Account{ID: accID, Name: in.Name}, nil
}
func (f *fakeAccountSvc) Get(context.Context, coretenant.Scope, string) (domain.Account, error) {
	return domain.Account{ID: accID, Name: "Budi", SourceType: "crm_contact", SourceID: "c1"}, f.getErr
}
func (f *fakeAccountSvc) List(context.Context, coretenant.Scope, string, int, int) ([]domain.Account, int64, error) {
	return []domain.Account{{ID: accID, Name: "Budi"}}, 1, nil
}

// settings service nyata di atas repo in-memory: batas 0–60 harus berlaku end-to-end.
type memSettingsRepo struct{ s domain.Settings }

func (m *memSettingsRepo) Get(context.Context, coretenant.Scope) (domain.Settings, error) {
	return m.s, nil
}
func (m *memSettingsRepo) Upsert(_ context.Context, _ coretenant.Scope, s domain.Settings, _ string) (domain.Settings, error) {
	m.s = s
	return s, nil
}

type noMembers struct{}

func (noMembers) Find(context.Context, coretenant.Scope, string) (service.Member, error) {
	return service.Member{UserID: "u1"}, nil
}
func (noMembers) WithPermission(context.Context, coretenant.Scope, string) ([]service.Member, error) {
	return nil, nil
}
func (noMembers) Active(context.Context, coretenant.Scope) ([]service.Member, error) {
	return []service.Member{{UserID: "u1", Name: "Sari", Email: "sari@example.com"}}, nil
}

type fixture struct {
	r        *gin.Engine
	invoices *fakeInvoices
	sends    *fakeSends
	payments *fakePayments
	accounts *fakeAccountSvc
}

func newFixture(t *testing.T, perms ...string) *fixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: "11111111-1111-1111-1111-111111111111", OrganizationSlug: "a", OrganizationType: coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive, MembershipID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", MembershipStatus: "active",
		MembershipVersion: 1, ResolutionSource: coretenant.ResolutionSourceSession, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{
		invoices: &fakeInvoices{invoice: domain.Invoice{ID: invID, InvoiceNumber: "INV-1", Status: domain.InvoiceIssued, GrandTotal: "100.00", AmountPaid: "0.00",
			Account: domain.Account{ID: accID, Name: "Budi"}, Items: []domain.InvoiceItem{{ID: "i1", Description: "Paket", Quantity: "1.00", UnitPrice: "100.00", LineTotal: "100.00"}}}},
		sends: &fakeSends{}, payments: &fakePayments{}, accounts: &fakeAccountSvc{},
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
		permissionmiddleware.SetUserID(c, "u1")
		c.Next()
	})
	checker := grant(perms...)
	group := r.Group("/app/receivable")
	NewAccountHandler(f.accounts).RegisterRoutes(group, checker)
	NewInvoiceHandler(f.invoices, f.sends).RegisterRoutes(group, checker)
	NewPaymentHandler(f.payments).RegisterRoutes(group, checker)
	NewSettingsHandler(service.NewSettingsService(&memSettingsRepo{s: domain.Settings{InvoiceLeadDays: 7, PaymentTermsDays: 7, DefaultChannels: []string{"email"}}}, noMembers{})).
		RegisterRoutes(group, checker)
	f.r = r
	return f
}

func call(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func base(path string) string { return "/app/receivable" + path }

// ---- RBAC ----

func TestRoutesRequirePermission(t *testing.T) {
	f := newFixture(t) // tidak ada izin sama sekali
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/accounts", ""}, {http.MethodPost, "/accounts", `{"name":"x"}`},
		{http.MethodGet, "/accounts/" + accID, ""}, {http.MethodPut, "/accounts/" + accID, `{"name":"x"}`},
		{http.MethodGet, "/invoices", ""}, {http.MethodPost, "/invoices", `{}`},
		{http.MethodGet, "/invoices/" + invID, ""}, {http.MethodPut, "/invoices/" + invID, `{}`},
		{http.MethodPost, "/invoices/" + invID + "/issue", ""}, {http.MethodPost, "/invoices/" + invID + "/void", `{"reason":"x"}`},
		{http.MethodGet, "/invoices/" + invID + "/pdf", ""}, {http.MethodPost, "/invoices/" + invID + "/link", ""},
		{http.MethodPost, "/invoices/" + invID + "/send", `{"channel":"email","client_request_id":"r"}`},
		{http.MethodGet, "/invoices/" + invID + "/sends", ""}, {http.MethodGet, "/invoices/" + invID + "/payments", ""},
		{http.MethodPost, "/invoices/" + invID + "/payments", `{"amount":"1"}`},
		{http.MethodGet, "/payments", ""}, {http.MethodGet, "/settings", ""}, {http.MethodPut, "/settings", `{}`},
		{http.MethodGet, "/members/senders", ""},
	} {
		if w := call(f.r, tc.method, base(tc.path), tc.body); w.Code != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", tc.method, tc.path, w.Code)
		}
	}
}

func TestPaymentRequiresMarkPaidPermission(t *testing.T) {
	f := newFixture(t, "invoice.read", "invoice.send", "invoice.update") // tanpa invoice.mark_paid
	if w := call(f.r, http.MethodPost, base("/invoices/"+invID+"/payments"), `{"amount":"100"}`); w.Code != http.StatusForbidden {
		t.Fatalf("status = %d", w.Code)
	}
	f = newFixture(t, "invoice.mark_paid")
	if w := call(f.r, http.MethodPost, base("/invoices/"+invID+"/payments"), `{"amount":"100"}`); w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body)
	}
}

func TestSendNeedsChannelPermissionOnTopOfInvoiceSend(t *testing.T) {
	body := func(ch string) string { return `{"channel":"` + ch + `","client_request_id":"r1"}` }
	f := newFixture(t, "invoice.send", "email.send") // tanpa whatsapp.message.send
	if w := call(f.r, http.MethodPost, base("/invoices/"+invID+"/send"), body("whatsapp")); w.Code != http.StatusForbidden || f.sends.sendCall != 0 {
		t.Fatalf("whatsapp without permission = %d (calls %d)", w.Code, f.sends.sendCall)
	}
	if w := call(f.r, http.MethodPost, base("/invoices/"+invID+"/send"), body("email")); w.Code != http.StatusOK || f.sends.sendCall != 1 {
		t.Fatalf("email = %d %s", w.Code, w.Body)
	}
	f = newFixture(t, "invoice.send", "whatsapp.message.send") // tanpa email.send
	if w := call(f.r, http.MethodPost, base("/invoices/"+invID+"/send"), body("email")); w.Code != http.StatusForbidden {
		t.Fatalf("email without permission = %d", w.Code)
	}
}

func TestSendersListAllowsSettingsOrInvoiceCreate(t *testing.T) {
	for _, perm := range []string{"receivable.settings", "invoice.create"} {
		f := newFixture(t, "invoice.read", perm)
		if w := call(f.r, http.MethodGet, base("/members/senders"), ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"user_id":"u1"`) {
			t.Fatalf("%s: %d %s", perm, w.Code, w.Body)
		}
	}
	f := newFixture(t, "invoice.read")
	if w := call(f.r, http.MethodGet, base("/members/senders"), ""); w.Code != http.StatusForbidden {
		t.Fatalf("read only = %d", w.Code)
	}
}

// ---- error mapping & boundary validation ----

func TestErrorMapping(t *testing.T) {
	f := newFixture(t, allReceivablePerms...)
	cases := []struct {
		name   string
		setup  func()
		method string
		path   string
		body   string
		status int
		code   string
	}{
		{"payment exceeds", func() { f.payments.err = repository.ErrPaymentExceeds }, "POST", "/invoices/" + invID + "/payments", `{"amount":"999"}`, 422, "PAYMENT_EXCEEDS_BALANCE"},
		{"payment invalid", func() { f.payments.err = service.ErrInvalidPayment }, "POST", "/invoices/" + invID + "/payments", `{"amount":"0"}`, 422, "VALIDATION_ERROR"},
		{"not payable", func() { f.payments.err = repository.ErrInvoiceNotPayable }, "POST", "/invoices/" + invID + "/payments", `{"amount":"1"}`, 409, "INVOICE_NOT_PAYABLE"},
		{"invoice missing", func() { f.invoices.getErr = receivable.ErrInvoiceNotFound }, "GET", "/invoices/" + invID, "", 404, "INVOICE_NOT_FOUND"},
		{"void with payments", func() { f.invoices.voidErr = repository.ErrInvoiceHasPayments }, "POST", "/invoices/" + invID + "/void", `{"reason":"x"}`, 409, "INVOICE_HAS_PAYMENTS"},
		{"void reason", func() { f.invoices.voidErr = service.ErrVoidReasonRequired }, "POST", "/invoices/" + invID + "/void", `{"reason":""}`, 422, "VALIDATION_ERROR"},
		{"not draft", func() { f.invoices.voidErr = repository.ErrInvoiceNotDraft }, "POST", "/invoices/" + invID + "/void", `{"reason":"x"}`, 409, "INVOICE_NOT_DRAFT"},
		{"channel unavailable", func() {
			f.sends.err = &service.ChannelError{Reason: "WhatsApp hanya tersedia untuk pelanggan yang terhubung ke kontak CRM."}
		}, "POST", "/invoices/" + invID + "/send", `{"channel":"whatsapp","client_request_id":"r"}`, 422, "CHANNEL_UNAVAILABLE"},
		{"not sendable", func() { f.sends.err = service.ErrInvoiceNotSendable }, "POST", "/invoices/" + invID + "/send", `{"channel":"email","client_request_id":"r"}`, 409, "INVOICE_NOT_SENDABLE"},
		{"account missing", func() { f.accounts.getErr = receivable.ErrAccountNotFound }, "GET", "/accounts/" + accID, "", 404, "ACCOUNT_NOT_FOUND"},
		{"pdf failed", func() { f.invoices.pdfErr = service.ErrDocumentsUnavailable }, "GET", "/invoices/" + invID + "/pdf", "", 502, "INVOICE_PDF_FAILED"},
	}
	for _, tc := range cases {
		f.payments.err, f.invoices.getErr, f.invoices.voidErr, f.invoices.pdfErr, f.sends.err, f.accounts.getErr = nil, nil, nil, nil, nil, nil
		tc.setup()
		w := call(f.r, tc.method, base(tc.path), tc.body)
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) {
			t.Errorf("%s: %d %s, want %d %s", tc.name, w.Code, w.Body, tc.status, tc.code)
		}
	}
}

func TestChannelErrorMessageIsPassedToUser(t *testing.T) {
	f := newFixture(t, allReceivablePerms...)
	f.sends.err = &service.ChannelError{Reason: "Belum ada pengirim. Atur PIC atau pengirim default di Pengaturan Penagihan."}
	w := call(f.r, http.MethodPost, base("/invoices/"+invID+"/send"), `{"channel":"email","client_request_id":"r"}`)
	if !strings.Contains(w.Body.String(), "Belum ada pengirim.") {
		t.Fatalf("body = %s", w.Body)
	}
}

func TestMalformedIDsAreRejectedBeforeReachingServices(t *testing.T) {
	f := newFixture(t, allReceivablePerms...)
	if w := call(f.r, http.MethodGet, base("/invoices/not-a-uuid"), ""); w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "INVOICE_NOT_FOUND") {
		t.Fatalf("bad invoice id = %d %s", w.Code, w.Body)
	}
	if w := call(f.r, http.MethodGet, base("/accounts/not-a-uuid"), ""); w.Code != http.StatusNotFound {
		t.Fatalf("bad account id = %d", w.Code)
	}
	body := `{"account_id":"not-a-uuid","items":[{"description":"x","unit_price":"1"}]}`
	if w := call(f.r, http.MethodPost, base("/invoices"), body); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad account_id = %d %s", w.Code, w.Body)
	}
	body = `{"account_id":"` + accID + `","pic_user_id":"nope","items":[{"description":"x","unit_price":"1"}]}`
	if w := call(f.r, http.MethodPost, base("/invoices"), body); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad pic = %d", w.Code)
	}
	body = `{"account_id":"` + accID + `","items":[{"description":"x","unit_price":"1","product_id":"nope"}]}`
	if w := call(f.r, http.MethodPost, base("/invoices"), body); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad product = %d", w.Code)
	}
	body = `{"account_id":"` + accID + `","period_start":"05/10/2026","items":[{"description":"x","unit_price":"1"}]}`
	if w := call(f.r, http.MethodPost, base("/invoices"), body); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad date = %d", w.Code)
	}
	if f.invoices.created.AccountID != "" {
		t.Fatal("invalid requests must not reach the service")
	}
}

// ---- request mapping ----

func TestCreateInvoiceMapsRequestAsManualInvoice(t *testing.T) {
	f := newFixture(t, allReceivablePerms...)
	body := `{"account_id":"` + accID + `","channels":["email","whatsapp"],"notes":"Catatan","period_start":"2026-10-01","period_end":"2026-10-31",
	  "items":[{"description":"Internet","quantity":"1","unit":"bulan","unit_price":"300000","tax_percent":"11","charge_type":"recurring","billing_frequency":"monthly","payment_timing":"prepaid","period_start":"2026-10-01","period_end":"2026-10-31"},
	           {"description":"Instalasi","unit_price":"33000"}]}`
	w := call(f.r, http.MethodPost, base("/invoices"), body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d %s", w.Code, w.Body)
	}
	in := f.invoices.created
	if in.AccountID != accID || len(in.Channels) != 2 || in.Notes != "Catatan" || in.PeriodStart == nil || in.PeriodStart.Format("2006-01-02") != "2026-10-01" || len(in.Lines) != 2 {
		t.Fatalf("input = %+v", in)
	}
	if in.SourceType != "" || in.SourceID != "" || in.IdempotencyKey != "" {
		t.Fatalf("HTTP must not set a source: %+v", in)
	}
	l := in.Lines[0]
	if l.Attrs != (pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid}) || l.TaxPercent != "11" ||
		l.PeriodEnd == nil || l.PeriodEnd.Format("2006-01-02") != "2026-10-31" || l.Unit != "bulan" {
		t.Fatalf("line = %+v", l)
	}
}

func TestUpdateInvoiceUsesSameMapping(t *testing.T) {
	f := newFixture(t, allReceivablePerms...)
	w := call(f.r, http.MethodPut, base("/invoices/"+invID), `{"account_id":"`+accID+`","items":[{"description":"x","unit_price":"1"}]}`)
	if w.Code != http.StatusOK || f.invoices.updated.AccountID != accID || len(f.invoices.updated.Lines) != 1 {
		t.Fatalf("status = %d %s input=%+v", w.Code, w.Body, f.invoices.updated)
	}
}

func TestIssueUsesActingUser(t *testing.T) {
	f := newFixture(t, allReceivablePerms...)
	if w := call(f.r, http.MethodPost, base("/invoices/"+invID+"/issue"), ""); w.Code != http.StatusOK || f.invoices.issuedBy != "u1" {
		t.Fatalf("status = %d issuedBy = %q", w.Code, f.invoices.issuedBy)
	}
}

func TestSendRequiresClientRequestID(t *testing.T) {
	f := newFixture(t, allReceivablePerms...)
	if w := call(f.r, http.MethodPost, base("/invoices/"+invID+"/send"), `{"channel":"email"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", w.Code)
	}
	if w := call(f.r, http.MethodPost, base("/invoices/"+invID+"/send"), `{"channel":"sms","client_request_id":"r"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad channel status = %d", w.Code)
	}
	long := strings.Repeat("x", 2001)
	if w := call(f.r, http.MethodPost, base("/invoices/"+invID+"/send"), `{"channel":"email","client_request_id":"r","message":"`+long+`"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("long message status = %d", w.Code)
	}
	if f.sends.sendCall != 0 {
		t.Fatal("invalid sends must not reach the service")
	}
	w := call(f.r, http.MethodPost, base("/invoices/"+invID+"/send"), `{"channel":"email","recipient":"x@y.id","message":"Halo","client_request_id":"r1"}`)
	if w.Code != http.StatusOK || f.sends.in.Recipient != "x@y.id" || f.sends.in.ClientRequestID != "r1" || f.sends.in.Message != "Halo" {
		t.Fatalf("send = %d %s in=%+v", w.Code, w.Body, f.sends.in)
	}
}

func TestRecordPaymentParsesDateAndReturnsInvoice(t *testing.T) {
	f := newFixture(t, allReceivablePerms...)
	w := call(f.r, http.MethodPost, base("/invoices/"+invID+"/payments"), `{"amount":"100","paid_at":"2026-10-05T10:00:00+07:00","reference":"TRF-1","note":"lunas"}`)
	if w.Code != http.StatusOK || f.payments.in.Amount != "100" || f.payments.in.Reference != "TRF-1" || f.payments.in.PaidAt.UTC().Format(time.RFC3339) != "2026-10-05T03:00:00Z" {
		t.Fatalf("status = %d %s in=%+v", w.Code, w.Body, f.payments.in)
	}
	if !strings.Contains(w.Body.String(), `"status":"paid"`) || !strings.Contains(w.Body.String(), `"amount":"100.00"`) {
		t.Fatalf("body = %s", w.Body)
	}
	if w := call(f.r, http.MethodPost, base("/invoices/"+invID+"/payments"), `{"amount":"100","paid_at":"kemarin"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad paid_at = %d", w.Code)
	}
	if w := call(f.r, http.MethodPost, base("/invoices/"+invID+"/payments"), `{"amount":"100","paid_at":"2026-10-05"}`); w.Code != http.StatusOK {
		t.Fatalf("date-only paid_at = %d %s", w.Code, w.Body)
	}
}

// ---- responses ----

func TestListPassesFiltersAndReturnsLastSendAndMeta(t *testing.T) {
	f := newFixture(t, allReceivablePerms...)
	f.invoices.listed = []domain.Invoice{{ID: invID, InvoiceNumber: "INV-1", Status: domain.InvoiceIssued, GrandTotal: "100.00", AmountPaid: "0.00", Account: domain.Account{Name: "Budi"}}}
	f.sends.last = map[string]domain.Send{invID: {ID: "s9", InvoiceID: invID, Channel: "email", Status: "failed", Error: "Belum ada pengirim."}}
	w := call(f.r, http.MethodGet, base("/invoices?status=issued&send_failed=true&search=Budi&account_id="+accID+"&page=2&per_page=10"), "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d %s", w.Code, w.Body)
	}
	fl := f.invoices.listFilter
	if fl.Status != "issued" || !fl.SendFailed || fl.Search != "Budi" || fl.AccountID != accID || fl.Limit != 10 || fl.Offset != 10 {
		t.Fatalf("filter = %+v", fl)
	}
	for _, want := range []string{`"last_send":{`, `"error":"Belum ada pengirim."`, `"balance":"100.00"`, `"page":2`, `"per_page":10`, `"total":1`, `"items":[]`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("missing %s in %s", want, w.Body)
		}
	}
	if w := call(f.r, http.MethodGet, base("/invoices?status=bogus"), ""); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad status filter = %d", w.Code)
	}
	if w := call(f.r, http.MethodGet, base("/invoices?account_id=nope"), ""); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad account filter = %d", w.Code)
	}
}

func TestInvoiceDetailPayload(t *testing.T) {
	f := newFixture(t, allReceivablePerms...)
	f.sends.list = []domain.Send{{ID: "s1", InvoiceID: invID, Channel: "email", Status: "sent", Recipient: "b@x.id"}}
	w := call(f.r, http.MethodGet, base("/invoices/"+invID), "")
	for _, want := range []string{`"invoice_number":"INV-1"`, `"status":"issued"`, `"balance":"100.00"`, `"description":"Paket"`, `"last_send":{`, `"recipient":"b@x.id"`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("missing %s in %s", want, w.Body)
		}
	}
	w = call(f.r, http.MethodGet, base("/invoices/"+invID+"/sends"), "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"channel":"email"`) {
		t.Fatalf("sends = %d %s", w.Code, w.Body)
	}
}

func TestPDFAndLinkResponses(t *testing.T) {
	f := newFixture(t, allReceivablePerms...)
	w := call(f.r, http.MethodGet, base("/invoices/"+invID+"/pdf"), "")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/pdf" || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("pdf = %d %v", w.Code, w.Header())
	}
	w = call(f.r, http.MethodPost, base("/invoices/"+invID+"/link"), "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"url":"https://app.test/i/tok"`) || !strings.Contains(w.Body.String(), `"expires_at"`) {
		t.Fatalf("link = %d %s", w.Code, w.Body)
	}
}

func TestPaymentListsAndAccounts(t *testing.T) {
	f := newFixture(t, allReceivablePerms...)
	w := call(f.r, http.MethodGet, base("/payments?page=3&per_page=10"), "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"invoice_number":"INV-1"`) || !strings.Contains(w.Body.String(), `"account_name":"Budi"`) ||
		!strings.Contains(w.Body.String(), `"total":25`) || !strings.Contains(w.Body.String(), `"total_pages":3`) {
		t.Fatalf("payments = %d %s", w.Code, w.Body)
	}
	w = call(f.r, http.MethodGet, base("/invoices/"+invID+"/payments"), "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"method":"manual"`) {
		t.Fatalf("invoice payments = %d %s", w.Code, w.Body)
	}
	w = call(f.r, http.MethodPost, base("/accounts"), `{"name":"Budi","company_name":"PT Maju","email":"b@x.id","phone":"0812","address":"Purbalingga"}`)
	if w.Code != http.StatusCreated || f.accounts.created.CompanyName != "PT Maju" || f.accounts.created.SourceType != "" || f.accounts.created.SourceID != "" {
		t.Fatalf("create account = %d %s input=%+v", w.Code, w.Body, f.accounts.created)
	}
	w = call(f.r, http.MethodGet, base("/accounts/"+accID), "")
	if !strings.Contains(w.Body.String(), `"contact_id":"c1"`) {
		t.Fatalf("account = %s", w.Body)
	}
}

func TestSettingsEndpoint(t *testing.T) {
	f := newFixture(t, allReceivablePerms...)
	if w := call(f.r, http.MethodPut, base("/settings"), `{"invoice_lead_days":61,"payment_terms_days":7,"default_channels":["email"]}`); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("lead days 61 = %d %s", w.Code, w.Body)
	}
	if w := call(f.r, http.MethodPut, base("/settings"), `{"invoice_lead_days":3,"payment_terms_days":14,"default_channels":["email","whatsapp"],"default_sender_user_id":"nope"}`); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad sender uuid = %d", w.Code)
	}
	w := call(f.r, http.MethodPut, base("/settings"), `{"invoice_lead_days":3,"payment_terms_days":14,"default_channels":["email","whatsapp"],"default_sender_user_id":"`+accID+`"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"payment_terms_days":14`) {
		t.Fatalf("update = %d %s", w.Code, w.Body)
	}
	w = call(f.r, http.MethodGet, base("/settings"), "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"invoice_lead_days":3`) {
		t.Fatalf("get = %d %s", w.Code, w.Body)
	}
}
