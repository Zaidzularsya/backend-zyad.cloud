package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type rotateCall struct {
	id, hash string
	dueAt    time.Time
}

type mgmtStoreFake struct {
	repository.ActivationRepository
	items      []repository.ActivationListItem
	total      int64
	byID       map[string]domain.LandingPageActivation
	listFilter repository.ActivationListFilter
	rotates    []rotateCall
	rotateErr  error
}

func (f *mgmtStoreFake) ListByOrganization(_ context.Context, _ coretenant.Scope, filter repository.ActivationListFilter) ([]repository.ActivationListItem, int64, error) {
	f.listFilter = filter
	return f.items, f.total, nil
}

func (f *mgmtStoreFake) FindByID(_ context.Context, scope coretenant.Scope, id string) (domain.LandingPageActivation, error) {
	a, ok := f.byID[id]
	if !ok || a.OrganizationID != scope.OrganizationID() {
		return domain.LandingPageActivation{}, pgx.ErrNoRows
	}
	return a, nil
}

func (f *mgmtStoreFake) RotatePreview(_ context.Context, _ coretenant.Scope, id, hash string, dueAt time.Time) (domain.LandingPageActivation, error) {
	if f.rotateErr != nil {
		return domain.LandingPageActivation{}, f.rotateErr
	}
	f.rotates = append(f.rotates, rotateCall{id, hash, dueAt})
	a := f.byID[id]
	a.PreviewTokenHash, a.InvoiceDueAt = hash, dueAt
	f.byID[id] = a
	return a, nil
}

type payLinksFake struct {
	urls map[string]string
	err  map[string]error
}

func (f payLinksFake) PaymentURL(_ context.Context, _ coretenant.Scope, invoiceID string) (string, error) {
	if err := f.err[invoiceID]; err != nil {
		return "", err
	}
	return f.urls[invoiceID], nil
}

type previewLinksFailing struct{ err error }

func (f previewLinksFailing) PreviewURL(context.Context, coretenant.Scope, string) (string, error) {
	return "", f.err
}

func strPtr(s string) *string { return &s }

func openAct(id, invoice string, dueIn time.Duration) domain.LandingPageActivation {
	a := domain.LandingPageActivation{
		ID: id, OrganizationID: landingServiceOrganizationID, PageID: "page-" + id,
		BuyerName: "Budi", BuyerContact: "081234567890", Price: "150000.00",
		PreviewTokenHash: "old-hash", InvoiceDueAt: activationTestNow.Add(dueIn),
		Status: domain.ActivationStatusOpen, CreatedAt: activationTestNow.Add(-2 * time.Hour),
	}
	if invoice != "" {
		a.InvoiceID = strPtr(invoice)
	}
	return a
}

type mgmtHarness struct {
	svc      ActivationManager
	store    *mgmtStoreFake
	features *activationFeaturesFake
}

func newMgmtHarness(links ActivationLinks) *mgmtHarness {
	h := &mgmtHarness{
		store:    &mgmtStoreFake{byID: map[string]domain.LandingPageActivation{}},
		features: &activationFeaturesFake{enabled: map[string]bool{"landing.enabled": true, "landing.page_activation": true}},
	}
	if links == nil {
		links = activationLinksFake{}
	}
	h.svc = NewActivationManager(ActivationManagerDeps{
		Activations: h.store, Features: h.features, Links: links,
		Payments: payLinksFake{urls: map[string]string{"inv-1": "https://app.test/i/PAY1", "inv-2": "https://app.test/i/PAY2"}},
		Now:      func() time.Time { return activationTestNow },
	})
	return h
}

func listItem(a domain.LandingPageActivation) repository.ActivationListItem {
	return repository.ActivationListItem{Activation: a, PageTitle: "Undangan Budi", PageSlug: "undangan-budi", PageStatus: domain.PageStatusDraft}
}

func TestActivationListDerivesStateAndPaymentLinks(t *testing.T) {
	h := newMgmtHarness(nil)
	paid := openAct("a-paid", "inv-2", -time.Hour)
	paid.Status, paid.PaidAt = domain.ActivationStatusPaid, &activationTestNow
	void := openAct("a-void", "inv-1", time.Hour)
	void.Status = domain.ActivationStatusVoid
	h.store.items = []repository.ActivationListItem{
		listItem(openAct("a-wait", "inv-1", time.Hour)),
		listItem(openAct("a-exp", "inv-1", -time.Hour)),
		listItem(paid),
		listItem(void),
	}
	h.store.total = 4

	got, err := h.svc.List(context.Background(), mustLandingScope(t), ActivationListQuery{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := map[string]struct{ state, payURL string }{
		"a-wait": {ActivationEntryAwaitingPayment, "https://app.test/i/PAY1"},
		"a-exp":  {ActivationEntryExpired, "https://app.test/i/PAY1"},
		"a-paid": {ActivationEntryPaid, "https://app.test/i/PAY2"},
		"a-void": {ActivationEntryVoid, ""},
	}
	if got.Total != 4 || len(got.Items) != 4 {
		t.Fatalf("total=%d items=%d", got.Total, len(got.Items))
	}
	for _, e := range got.Items {
		w := want[e.Activation.ID]
		if e.State != w.state || e.PaymentURL != w.payURL {
			t.Fatalf("%s: state=%q pay=%q, want %q/%q", e.Activation.ID, e.State, e.PaymentURL, w.state, w.payURL)
		}
		if e.PageTitle != "Undangan Budi" || e.PageSlug != "undangan-budi" {
			t.Fatalf("info halaman hilang: %+v", e)
		}
	}
}

func TestActivationListPagingAndStatusFilter(t *testing.T) {
	h := newMgmtHarness(nil)
	if _, err := h.svc.List(context.Background(), mustLandingScope(t), ActivationListQuery{}); err != nil {
		t.Fatal(err)
	}
	if h.store.listFilter.Limit != 20 || h.store.listFilter.Offset != 0 || h.store.listFilter.Status != "" {
		t.Fatalf("default filter = %+v", h.store.listFilter)
	}
	if _, err := h.svc.List(context.Background(), mustLandingScope(t), ActivationListQuery{Status: "paid", Page: 3, PerPage: 500}); err != nil {
		t.Fatal(err)
	}
	if h.store.listFilter.Status != domain.ActivationStatusPaid || h.store.listFilter.Limit != 100 || h.store.listFilter.Offset != 200 {
		t.Fatalf("filter = %+v (perPage dibatasi 100, offset=(3-1)*100)", h.store.listFilter)
	}
	_, err := h.svc.List(context.Background(), mustLandingScope(t), ActivationListQuery{Status: "bogus"})
	requireAppError(t, err, "VALIDATION_ERROR", http.StatusUnprocessableEntity)
}

func TestActivationListPaymentLinkFailureDoesNotBreakList(t *testing.T) {
	h := newMgmtHarness(nil)
	h.svc = NewActivationManager(ActivationManagerDeps{
		Activations: h.store, Features: h.features, Links: activationLinksFake{},
		Payments: payLinksFake{err: map[string]error{"inv-1": errors.New("boom")}},
		Now:      func() time.Time { return activationTestNow },
	})
	h.store.items = []repository.ActivationListItem{listItem(openAct("a1", "inv-1", time.Hour))}
	got, err := h.svc.List(context.Background(), mustLandingScope(t), ActivationListQuery{})
	if err != nil || len(got.Items) != 1 || got.Items[0].PaymentURL != "" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestActivationListDoesNotRequireSellingEntitlements(t *testing.T) {
	h := newMgmtHarness(nil)
	h.features.enabled = map[string]bool{} // fitur reseller dicabut: riwayat tetap bisa dibaca
	if _, err := h.svc.List(context.Background(), mustLandingScope(t), ActivationListQuery{}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
}

func TestActivationRenewRotatesTokenAndExtendsPreviewWindow(t *testing.T) {
	h := newMgmtHarness(nil)
	h.store.byID["a1"] = openAct("a1", "inv-1", -3*time.Hour) // sudah lewat jatuh tempo
	got, err := h.svc.Renew(context.Background(), mustLandingScope(t), "a1", ActivationRenewInput{InvoiceDueDays: intPtr(2)}, "user-1")
	if err != nil {
		t.Fatalf("Renew() error = %v", err)
	}
	if len(h.store.rotates) != 1 {
		t.Fatalf("rotates = %v", h.store.rotates)
	}
	call := h.store.rotates[0]
	if call.id != "a1" || call.dueAt != activationTestNow.Add(48*time.Hour) {
		t.Fatalf("rotate = %+v, want due now+48h", call)
	}
	const prefix = "https://reseller.example.test/preview/"
	if !strings.HasPrefix(got.PreviewURL, prefix) {
		t.Fatalf("preview url = %q", got.PreviewURL)
	}
	rawToken := strings.TrimPrefix(got.PreviewURL, prefix)
	if domain.HashActivationPreviewToken(rawToken) != call.hash || call.hash == "old-hash" {
		t.Fatal("hash yang disimpan harus hash dari token baru di link, bukan token lama")
	}
	if got.PaymentURL != "https://app.test/i/PAY1" || got.Activation.InvoiceDueAt != call.dueAt {
		t.Fatalf("result = %+v", got)
	}
}

func TestActivationRenewDefaultsAndBounds(t *testing.T) {
	h := newMgmtHarness(nil)
	h.store.byID["a1"] = openAct("a1", "inv-1", time.Hour)
	if _, err := h.svc.Renew(context.Background(), mustLandingScope(t), "a1", ActivationRenewInput{}, "u"); err != nil {
		t.Fatal(err)
	}
	if h.store.rotates[0].dueAt != activationTestNow.Add(24*time.Hour) {
		t.Fatalf("default harus 24 jam, got %v", h.store.rotates[0].dueAt)
	}
	for name, in := range map[string]ActivationRenewInput{
		"jam dan hari bersamaan": {InvoiceDueHours: intPtr(2), InvoiceDueDays: intPtr(1)},
		"jam di luar batas":      {InvoiceDueHours: intPtr(721)},
		"hari nol":               {InvoiceDueDays: intPtr(0)},
	} {
		_, err := h.svc.Renew(context.Background(), mustLandingScope(t), "a1", in, "u")
		if err == nil {
			t.Fatalf("%s: harus ditolak", name)
		}
		requireAppError(t, err, "VALIDATION_ERROR", http.StatusUnprocessableEntity)
	}
}

func TestActivationRenewRejectsUnknownAndNotOpen(t *testing.T) {
	h := newMgmtHarness(nil)
	paid := openAct("a-paid", "inv-1", time.Hour)
	paid.Status = domain.ActivationStatusPaid
	void := openAct("a-void", "inv-1", time.Hour)
	void.Status = domain.ActivationStatusVoid
	other := openAct("a-other", "inv-1", time.Hour)
	other.OrganizationID = "99999999-9999-9999-9999-999999999999"
	h.store.byID = map[string]domain.LandingPageActivation{
		"a-paid": paid, "a-void": void, "a-other": other, "a-noinv": openAct("a-noinv", "", time.Hour),
	}
	scope := mustLandingScope(t)

	_, err := h.svc.Renew(context.Background(), scope, "tidak-ada", ActivationRenewInput{}, "u")
	requireAppError(t, err, "ACTIVATION_NOT_FOUND", http.StatusNotFound)
	_, err = h.svc.Renew(context.Background(), scope, "a-other", ActivationRenewInput{}, "u")
	requireAppError(t, err, "ACTIVATION_NOT_FOUND", http.StatusNotFound)
	for _, id := range []string{"a-paid", "a-void", "a-noinv"} {
		_, err = h.svc.Renew(context.Background(), scope, id, ActivationRenewInput{}, "u")
		requireAppError(t, err, "ACTIVATION_NOT_OPEN", http.StatusConflict)
	}
	if len(h.store.rotates) != 0 {
		t.Fatalf("tidak boleh memutar token: %v", h.store.rotates)
	}
}

func TestActivationRenewRequiresSellingEntitlements(t *testing.T) {
	h := newMgmtHarness(nil)
	h.store.byID["a1"] = openAct("a1", "inv-1", time.Hour)
	h.features.enabled = map[string]bool{"landing.enabled": true}
	_, err := h.svc.Renew(context.Background(), mustLandingScope(t), "a1", ActivationRenewInput{}, "u")
	requireAppError(t, err, "FEATURE_NOT_ENABLED", http.StatusForbidden)
	if len(h.store.rotates) != 0 {
		t.Fatal("tanpa fitur tidak boleh memutar token")
	}
}

func TestActivationRenewWithoutPublicHostDoesNotRotate(t *testing.T) {
	linkErr := errors.New("no host")
	h := newMgmtHarness(previewLinksFailing{err: linkErr})
	h.store.byID["a1"] = openAct("a1", "inv-1", time.Hour)
	if _, err := h.svc.Renew(context.Background(), mustLandingScope(t), "a1", ActivationRenewInput{}, "u"); !errors.Is(err, linkErr) {
		t.Fatalf("err = %v, want link error", err)
	}
	if len(h.store.rotates) != 0 {
		t.Fatal("link preview gagal dibentuk: token lama tidak boleh diganti")
	}
}

func TestActivationRenewLostRaceIsConflict(t *testing.T) {
	h := newMgmtHarness(nil)
	h.store.byID["a1"] = openAct("a1", "inv-1", time.Hour)
	h.store.rotateErr = pgx.ErrNoRows // dibayar/dibatalkan tepat sebelum update
	_, err := h.svc.Renew(context.Background(), mustLandingScope(t), "a1", ActivationRenewInput{}, "u")
	requireAppError(t, err, "ACTIVATION_NOT_OPEN", http.StatusConflict)
}
