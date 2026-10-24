package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

// activationStoreFake models landing_page_activations with the one-open-per-page
// rule and the overdue replacement of CreateReplacingOverdue.
type activationStoreFake struct {
	repository.ActivationRepository
	rows       map[string]*domain.LandingPageActivation
	seq        int
	createErr  error
	setInvErr  error
	voidCalls  []string
	createSeen []repository.CreateActivationParams
}

func newActivationStoreFake() *activationStoreFake {
	return &activationStoreFake{rows: map[string]*domain.LandingPageActivation{}}
}

func (f *activationStoreFake) openFor(orgID, pageID string) *domain.LandingPageActivation {
	for _, a := range f.rows {
		if a.OrganizationID == orgID && a.PageID == pageID && a.Status == domain.ActivationStatusOpen {
			return a
		}
	}
	return nil
}

func (f *activationStoreFake) FindOpenByPage(_ context.Context, scope coretenant.Scope, pageID string) (domain.LandingPageActivation, error) {
	if a := f.openFor(scope.OrganizationID(), pageID); a != nil {
		return *a, nil
	}
	return domain.LandingPageActivation{}, pgx.ErrNoRows
}

func (f *activationStoreFake) CreateReplacingOverdue(_ context.Context, scope coretenant.Scope, p repository.CreateActivationParams, now time.Time) (domain.LandingPageActivation, *domain.LandingPageActivation, error) {
	f.createSeen = append(f.createSeen, p)
	if f.createErr != nil {
		return domain.LandingPageActivation{}, nil, f.createErr
	}
	var voided *domain.LandingPageActivation
	if open := f.openFor(scope.OrganizationID(), p.PageID); open != nil {
		if open.InvoiceDueAt.After(now) {
			return domain.LandingPageActivation{}, nil, repository.ErrActivationOpen
		}
		open.Status = domain.ActivationStatusVoid
		v := *open
		voided = &v
	}
	f.seq++
	a := &domain.LandingPageActivation{
		ID: "act-" + string(rune('0'+f.seq)), OrganizationID: scope.OrganizationID(), PageID: p.PageID,
		BuyerName: p.BuyerName, BuyerContact: p.BuyerContact, BuyerEmail: p.BuyerEmail, Price: p.Price,
		DurationValue: p.DurationValue, DurationUnit: p.DurationUnit, PreviewTokenHash: p.PreviewTokenHash,
		InvoiceDueAt: p.InvoiceDueAt, Status: domain.ActivationStatusOpen, CreatedAt: now,
	}
	f.rows[a.ID] = a
	return *a, voided, nil
}

func (f *activationStoreFake) SetInvoice(_ context.Context, scope coretenant.Scope, id, invoiceID string) (domain.LandingPageActivation, error) {
	if f.setInvErr != nil {
		return domain.LandingPageActivation{}, f.setInvErr
	}
	a, ok := f.rows[id]
	if !ok || a.OrganizationID != scope.OrganizationID() || a.Status != domain.ActivationStatusOpen || a.InvoiceID != nil {
		return domain.LandingPageActivation{}, pgx.ErrNoRows
	}
	a.InvoiceID = &invoiceID
	return *a, nil
}

func (f *activationStoreFake) Void(_ context.Context, scope coretenant.Scope, id string) (domain.LandingPageActivation, error) {
	f.voidCalls = append(f.voidCalls, id)
	a, ok := f.rows[id]
	if !ok || a.OrganizationID != scope.OrganizationID() || a.Status != domain.ActivationStatusOpen {
		return domain.LandingPageActivation{}, pgx.ErrNoRows
	}
	a.Status = domain.ActivationStatusVoid
	return *a, nil
}

type activationFeaturesFake struct {
	enabled map[string]bool
	err     error
	asked   []string
}

func (f *activationFeaturesFake) RequireFeature(_ context.Context, orgID, key string) error {
	f.asked = append(f.asked, orgID+"|"+key)
	if f.err != nil {
		return f.err
	}
	if !f.enabled[key] {
		return coreerrors.New("FEATURE_NOT_ENABLED", "billing feature is not enabled", http.StatusForbidden)
	}
	return nil
}

type activationBillingFake struct {
	requests []ActivationInvoiceRequest
	scopes   []string
	fail     error
	failID   string // invoice id returned together with fail
	voided   []string
	n        int
}

func (f *activationBillingFake) IssueInvoice(_ context.Context, scope coretenant.Scope, req ActivationInvoiceRequest, _ string) (ActivationInvoice, error) {
	f.requests = append(f.requests, req)
	f.scopes = append(f.scopes, scope.OrganizationID())
	if f.fail != nil {
		return ActivationInvoice{InvoiceID: f.failID}, f.fail
	}
	f.n++
	id := "inv-" + string(rune('0'+f.n))
	return ActivationInvoice{InvoiceID: id, PaymentURL: "https://app.example.test/i/link-" + id}, nil
}

func (f *activationBillingFake) VoidInvoice(_ context.Context, _ coretenant.Scope, invoiceID, _, _ string) error {
	f.voided = append(f.voided, invoiceID)
	return nil
}

type activationLinksFake struct{}

func (activationLinksFake) PreviewURL(_ context.Context, _ coretenant.Scope, token string) (string, error) {
	return "https://reseller.example.test/api/v1/public/landing/preview/" + token, nil
}

type activationHarness struct {
	svc      ActivationService
	store    *activationStoreFake
	features *activationFeaturesFake
	billing  *activationBillingFake
	pages    *activationPageRepoFake
}

func newActivationHarness() *activationHarness {
	h := &activationHarness{
		store:    newActivationStoreFake(),
		features: &activationFeaturesFake{enabled: map[string]bool{"landing.enabled": true, "landing.page_activation": true}},
		billing:  &activationBillingFake{},
		pages: &activationPageRepoFake{pages: map[string]domain.LandingPage{
			"page-a": {ID: "page-a", OrganizationID: landingServiceOrganizationID, Slug: "undangan-budi",
				Builder: domain.PageBuilderGrapesJS, Status: domain.PageStatusDraft},
			"page-other-org": {ID: "page-other-org", OrganizationID: "99999999-9999-9999-9999-999999999999", Slug: "x",
				Builder: domain.PageBuilderGrapesJS, Status: domain.PageStatusDraft},
			"page-sections": {ID: "page-sections", OrganizationID: landingServiceOrganizationID, Slug: "lama",
				Builder: domain.PageBuilderSections, Status: domain.PageStatusDraft},
			"page-template": {ID: "page-template", OrganizationID: landingServiceOrganizationID, Slug: "tpl",
				Builder: domain.PageBuilderGrapesJS, Status: domain.PageStatusDraft, IsTemplate: true},
			"page-archived": {ID: "page-archived", OrganizationID: landingServiceOrganizationID, Slug: "arsip",
				Builder: domain.PageBuilderGrapesJS, Status: domain.PageStatusArchived},
		}},
	}
	h.svc = NewActivationService(ActivationServiceDeps{
		Activations: h.store, Pages: h.pages, Features: h.features, Billing: h.billing, Links: activationLinksFake{},
		Now: func() time.Time { return activationTestNow },
	})
	return h
}

func validActivationInput() ActivationInput {
	return ActivationInput{
		BuyerName: "Budi", BuyerContact: "+6281234567890", BuyerEmail: "budi@example.test",
		Price: "150000", DurationValue: 1, DurationUnit: domain.ActivationDurationMonth,
	}
}

func intPtr(v int) *int { return &v }

func requireAppError(t *testing.T, err error, code string, status int) {
	t.Helper()
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("err = %v, want AppError %s", err, code)
	}
	if appErr.Code != code || appErr.Status != status {
		t.Fatalf("err = %s/%d (%s), want %s/%d", appErr.Code, appErr.Status, appErr.Message, code, status)
	}
}

func TestActivationCreateInvoice_Success(t *testing.T) {
	h := newActivationHarness()
	res, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", validActivationInput(), "user-1")
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	a := res.Activation
	if a.Status != domain.ActivationStatusOpen || a.InvoiceID == nil || *a.InvoiceID != "inv-1" {
		t.Fatalf("activation = %+v", a)
	}
	if a.Price != "150000.00" || a.DurationValue != 1 || a.DurationUnit != domain.ActivationDurationMonth {
		t.Fatalf("activation fields = %+v", a)
	}
	if !a.InvoiceDueAt.Equal(activationTestNow.Add(time.Hour)) {
		t.Fatalf("invoice_due_at = %s, want now+1h default", a.InvoiceDueAt)
	}
	// Preview URL carries the raw token whose hash is the stored one.
	const prefix = "https://reseller.example.test/api/v1/public/landing/preview/"
	if !strings.HasPrefix(res.PreviewURL, prefix) {
		t.Fatalf("PreviewURL = %q", res.PreviewURL)
	}
	token := strings.TrimPrefix(res.PreviewURL, prefix)
	if len(token) < 43 || domain.HashActivationPreviewToken(token) != a.PreviewTokenHash {
		t.Fatalf("token %q does not match stored hash", token)
	}
	if res.PaymentURL != "https://app.example.test/i/link-inv-1" {
		t.Fatalf("PaymentURL = %q", res.PaymentURL)
	}
	// Invoice in the reseller's own org, keyed by the activation, one line for the price.
	if len(h.billing.requests) != 1 || h.billing.scopes[0] != landingServiceOrganizationID {
		t.Fatalf("billing calls = %+v / %v", h.billing.requests, h.billing.scopes)
	}
	req := h.billing.requests[0]
	if req.ActivationID != a.ID || req.Description != "Halaman undangan undangan-budi" || req.Price != "150000.00" ||
		!req.DueAt.Equal(a.InvoiceDueAt) || req.BuyerName != "Budi" || req.BuyerEmail != "budi@example.test" ||
		req.BuyerContact != "+6281234567890" {
		t.Fatalf("invoice request = %+v", req)
	}
	if strings.Contains(req.Description, token) {
		t.Fatal("raw token must not reach the invoice")
	}
	for _, key := range []string{"landing.enabled", "landing.page_activation"} {
		found := false
		for _, asked := range h.features.asked {
			if asked == landingServiceOrganizationID+"|"+key {
				found = true
			}
		}
		if !found {
			t.Fatalf("feature %s not checked for the scope org: %v", key, h.features.asked)
		}
	}
}

func TestActivationCreateInvoice_RequiresEntitlements(t *testing.T) {
	for _, missing := range []string{"landing.page_activation", "landing.enabled"} {
		h := newActivationHarness()
		h.features.enabled[missing] = false
		_, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", validActivationInput(), "user-1")
		requireAppError(t, err, "FEATURE_NOT_ENABLED", http.StatusForbidden)
		if len(h.store.createSeen) != 0 || len(h.billing.requests) != 0 {
			t.Fatalf("%s missing: nothing must be created", missing)
		}
	}
}

func TestActivationCreateInvoice_FeatureCheckErrorFailsClosed(t *testing.T) {
	h := newActivationHarness()
	h.features.err = errors.New("db down")
	if _, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", validActivationInput(), "u"); err == nil {
		t.Fatal("want error")
	}
	if len(h.store.createSeen) != 0 {
		t.Fatal("nothing must be created when the entitlement check fails")
	}

	nilGate := NewActivationService(ActivationServiceDeps{Activations: h.store, Pages: h.pages, Billing: h.billing})
	_, err := nilGate.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", validActivationInput(), "u")
	requireAppError(t, err, "FEATURE_NOT_ENABLED", http.StatusForbidden)
}

func TestActivationCreateInvoice_PageNotInScopeIs404(t *testing.T) {
	for _, id := range []string{"page-other-org", "missing", "page-template"} {
		h := newActivationHarness()
		_, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), id, validActivationInput(), "u")
		requireAppError(t, err, "PAGE_NOT_FOUND", http.StatusNotFound)
		if len(h.store.createSeen) != 0 {
			t.Fatalf("%s: nothing must be created", id)
		}
	}
}

func TestActivationCreateInvoice_PageNotEligible(t *testing.T) {
	for _, id := range []string{"page-sections", "page-archived"} {
		h := newActivationHarness()
		_, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), id, validActivationInput(), "u")
		requireAppError(t, err, "ACTIVATION_PAGE_NOT_ELIGIBLE", http.StatusUnprocessableEntity)
	}
}

func TestActivationCreateInvoice_SecondWhileOpenIsConflict(t *testing.T) {
	h := newActivationHarness()
	if _, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", validActivationInput(), "u"); err != nil {
		t.Fatal(err)
	}
	_, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", validActivationInput(), "u")
	requireAppError(t, err, "ACTIVATION_OPEN", http.StatusConflict)
	if len(h.billing.requests) != 1 {
		t.Fatalf("second call must not issue an invoice, got %d", len(h.billing.requests))
	}
}

func TestActivationCreateInvoice_OverdueOpenIsVoidedAndReplaced(t *testing.T) {
	h := newActivationHarness()
	old := &domain.LandingPageActivation{
		ID: "act-old", OrganizationID: landingServiceOrganizationID, PageID: "page-a",
		Status: domain.ActivationStatusOpen, InvoiceDueAt: activationTestNow, // due exactly now = overdue
	}
	h.store.rows[old.ID] = old
	res, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", validActivationInput(), "u")
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if old.Status != domain.ActivationStatusVoid {
		t.Fatalf("old activation status = %s, want void", old.Status)
	}
	if res.Activation.ID == "act-old" || res.Activation.Status != domain.ActivationStatusOpen {
		t.Fatalf("new activation = %+v", res.Activation)
	}
	if len(h.billing.voided) != 0 {
		t.Fatal("the old invoice must not be touched (late payment stays acceptable)")
	}
}

func TestActivationCreateInvoice_InvalidInput(t *testing.T) {
	cases := map[string]func(*ActivationInput){
		"price zero":       func(in *ActivationInput) { in.Price = "0" },
		"price negative":   func(in *ActivationInput) { in.Price = "-5000" },
		"price float junk": func(in *ActivationInput) { in.Price = "1e5" },
		"price 3 decimals": func(in *ActivationInput) { in.Price = "10.123" },
		"price too big":    func(in *ActivationInput) { in.Price = "10000000000000000" },
		"no buyer name":    func(in *ActivationInput) { in.BuyerName = "  " },
		"no contact":       func(in *ActivationInput) { in.BuyerContact = "" },
		"bad email":        func(in *ActivationInput) { in.BuyerEmail = "not-an-email" },
		"duration zero":    func(in *ActivationInput) { in.DurationValue = 0 },
		"duration huge":    func(in *ActivationInput) { in.DurationValue = 1000 },
		"bad unit":         func(in *ActivationInput) { in.DurationUnit = "year" },
		"both due fields":  func(in *ActivationInput) { in.InvoiceDueHours, in.InvoiceDueDays = intPtr(1), intPtr(1) },
		"due hours zero":   func(in *ActivationInput) { in.InvoiceDueHours = intPtr(0) },
		"due hours > 30d":  func(in *ActivationInput) { in.InvoiceDueHours = intPtr(721) },
		"due days > 30":    func(in *ActivationInput) { in.InvoiceDueDays = intPtr(31) },
		"due days zero":    func(in *ActivationInput) { in.InvoiceDueDays = intPtr(0) },
	}
	for name, mutate := range cases {
		h := newActivationHarness()
		in := validActivationInput()
		mutate(&in)
		_, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", in, "u")
		var appErr *coreerrors.AppError
		if !errors.As(err, &appErr) || appErr.Status != http.StatusUnprocessableEntity {
			t.Fatalf("%s: err = %v, want 422", name, err)
		}
		if len(h.store.createSeen) != 0 {
			t.Fatalf("%s: nothing must be created", name)
		}
	}
}

func TestActivationCreateInvoice_DueMapping(t *testing.T) {
	cases := []struct {
		name  string
		hours *int
		days  *int
		want  time.Duration
	}{
		{"default 1 hour", nil, nil, time.Hour},
		{"hours=5", intPtr(5), nil, 5 * time.Hour},
		{"days=3", nil, intPtr(3), 72 * time.Hour},
		{"days=30", nil, intPtr(30), 30 * 24 * time.Hour},
		{"hours=720", intPtr(720), nil, 720 * time.Hour},
	}
	for _, tc := range cases {
		h := newActivationHarness()
		in := validActivationInput()
		in.InvoiceDueHours, in.InvoiceDueDays = tc.hours, tc.days
		res, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", in, "u")
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !res.Activation.InvoiceDueAt.Equal(activationTestNow.Add(tc.want)) {
			t.Fatalf("%s: invoice_due_at = %s, want now+%s", tc.name, res.Activation.InvoiceDueAt, tc.want)
		}
		if !h.billing.requests[0].DueAt.Equal(res.Activation.InvoiceDueAt) {
			t.Fatalf("%s: invoice due %s != activation due", tc.name, h.billing.requests[0].DueAt)
		}
	}
}

func TestActivationCreateInvoice_InvoiceFailureVoidsActivationAndRetryWorks(t *testing.T) {
	h := newActivationHarness()
	h.billing.fail = coreerrors.New("QUOTA_EXCEEDED", "billing quota has been exceeded", http.StatusConflict)
	_, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", validActivationInput(), "u")
	requireAppError(t, err, "QUOTA_EXCEEDED", http.StatusConflict)
	if len(h.store.voidCalls) != 1 {
		t.Fatalf("activation must be voided after invoice failure, void calls = %v", h.store.voidCalls)
	}
	if h.store.openFor(landingServiceOrganizationID, "page-a") != nil {
		t.Fatal("no open activation must remain")
	}

	h.billing.fail = nil
	res, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", validActivationInput(), "u")
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if res.Activation.InvoiceID == nil || res.Activation.Status != domain.ActivationStatusOpen {
		t.Fatalf("retry activation = %+v", res.Activation)
	}
	if h.billing.requests[0].ActivationID == h.billing.requests[1].ActivationID {
		t.Fatal("retry must use a new activation (new idempotency key)")
	}
}

func TestActivationCreateInvoice_InvoiceCreatedButFailedIsCancelled(t *testing.T) {
	h := newActivationHarness()
	h.billing.fail, h.billing.failID = errors.New("link failed"), "inv-orphan"
	if _, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", validActivationInput(), "u"); err == nil {
		t.Fatal("want error")
	}
	if len(h.billing.voided) != 1 || h.billing.voided[0] != "inv-orphan" {
		t.Fatalf("issued-but-failed invoice must be voided, got %v", h.billing.voided)
	}
	if len(h.store.voidCalls) != 1 {
		t.Fatal("activation must be voided")
	}
}

func TestActivationCreateInvoice_LinkFailureVoidsBoth(t *testing.T) {
	h := newActivationHarness()
	h.store.setInvErr = errors.New("db down")
	if _, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", validActivationInput(), "u"); err == nil {
		t.Fatal("want error")
	}
	if len(h.billing.voided) != 1 || h.billing.voided[0] != "inv-1" {
		t.Fatalf("unlinked invoice must be voided, got %v", h.billing.voided)
	}
	if h.store.openFor(landingServiceOrganizationID, "page-a") != nil {
		t.Fatal("activation must be voided")
	}
}

func TestActivationCreateInvoice_DefaultPreviewURLIsRelativePath(t *testing.T) {
	h := newActivationHarness()
	svc := NewActivationService(ActivationServiceDeps{
		Activations: h.store, Pages: h.pages, Features: h.features, Billing: h.billing,
		Now: func() time.Time { return activationTestNow },
	})
	res, err := svc.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", validActivationInput(), "u")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.PreviewURL, "/api/v1/public/landing/preview/") || strings.Contains(res.PreviewURL, "{token}") {
		t.Fatalf("PreviewURL = %q", res.PreviewURL)
	}
}

func TestActivationCreateInvoice_NormalizesOptionalEmail(t *testing.T) {
	h := newActivationHarness()
	in := validActivationInput()
	in.BuyerEmail = "  "
	res, err := h.svc.CreateInvoice(context.Background(), mustLandingScope(t), "page-a", in, "u")
	if err != nil {
		t.Fatal(err)
	}
	if res.Activation.BuyerEmail != nil || h.billing.requests[0].BuyerEmail != "" {
		t.Fatalf("blank email must be stored as NULL, got %v", res.Activation.BuyerEmail)
	}
}
