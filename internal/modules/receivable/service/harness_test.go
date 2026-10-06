package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/shared/docpdf"
	"zyad.cloud/internal/shared/pricing"
	"zyad.cloud/internal/shared/publiclink"
)

var ctx = context.Background()
var scope = coretenant.Scope{}

// nowWIB = 2026-10-05 10:00 WIB.
var nowWIB = time.Date(2026, 10, 5, 3, 0, 0, 0, time.UTC)

func day(s string) *time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return &d
}

// ---- fake store: InvoiceRepository + PaymentRepository di atas satu peta ----

type fakeStore struct {
	invoices    map[string]domain.Invoice
	payments    []domain.Payment
	seq         int
	issueRace   bool // Issue gagal ErrInvoiceNotDraft seolah request lain menang
	overdueSeen time.Time
}

func newFakeStore() *fakeStore { return &fakeStore{invoices: map[string]domain.Invoice{}} }

func (s *fakeStore) Create(_ context.Context, _ coretenant.Scope, p repository.CreateInvoiceParams) (domain.Invoice, error) {
	if p.SourceID != "" && p.IdempotencyKey != "" {
		for _, inv := range s.invoices {
			if string(inv.SourceType) == p.SourceType && inv.SourceID == p.SourceID && inv.IdempotencyKey == p.IdempotencyKey {
				return domain.Invoice{}, repository.ErrDuplicateSource
			}
		}
	}
	s.seq++
	st := domain.SourceType(p.SourceType)
	if st == "" {
		st = domain.SourceManual
	}
	inv := domain.Invoice{
		ID: fmt.Sprintf("inv-%d", s.seq), AccountID: p.AccountID, Account: domain.Account{ID: p.AccountID, Name: "Budi"},
		Status: domain.InvoiceDraft, SourceType: st, SourceID: p.SourceID, ContractID: p.ContractID, IdempotencyKey: p.IdempotencyKey,
		PeriodStart: p.PeriodStart, PeriodEnd: p.PeriodEnd, Currency: p.Currency, Subtotal: p.Totals.Subtotal,
		DiscountTotal: p.Totals.DiscountTotal, TaxTotal: p.Totals.TaxTotal, GrandTotal: p.Totals.GrandTotal, AmountPaid: "0.00",
		Channels: p.Channels, PICUserID: p.PICUserID, Notes: p.Notes, CreatedBy: p.CreatedBy,
	}
	for i, it := range p.Items {
		inv.Items = append(inv.Items, domain.InvoiceItem{ContractItemID: it.ContractItemID, Description: it.Line.Description, LineTotal: it.Line.LineTotal, Pricing: it.Line.Attrs, PeriodStart: it.PeriodStart, PeriodEnd: it.PeriodEnd, Position: i})
	}
	s.invoices[inv.ID] = inv
	return inv, nil
}

func (s *fakeStore) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.Invoice, error) {
	inv, ok := s.invoices[id]
	if !ok {
		return domain.Invoice{}, pgx.ErrNoRows
	}
	return inv, nil
}

func (s *fakeStore) FindBySource(_ context.Context, _ coretenant.Scope, sourceType, sourceID, key string) (domain.Invoice, error) {
	for _, inv := range s.invoices {
		if string(inv.SourceType) == sourceType && inv.SourceID == sourceID && inv.IdempotencyKey == key {
			return inv, nil
		}
	}
	return domain.Invoice{}, pgx.ErrNoRows
}

func (s *fakeStore) List(_ context.Context, _ coretenant.Scope, f repository.InvoiceListFilter) ([]domain.Invoice, int64, error) {
	out := []domain.Invoice{}
	for _, inv := range s.invoices {
		if f.Status == "" || string(inv.Status) == f.Status {
			out = append(out, inv)
		}
	}
	return out, int64(len(out)), nil
}

func (s *fakeStore) ReplaceDraft(_ context.Context, _ coretenant.Scope, id string, p repository.CreateInvoiceParams) (domain.Invoice, error) {
	inv, ok := s.invoices[id]
	if !ok {
		return domain.Invoice{}, pgx.ErrNoRows
	}
	if inv.Status != domain.InvoiceDraft {
		return domain.Invoice{}, repository.ErrInvoiceNotDraft
	}
	inv.AccountID, inv.Notes, inv.GrandTotal, inv.Channels = p.AccountID, p.Notes, p.Totals.GrandTotal, p.Channels
	s.invoices[id] = inv
	return inv, nil
}

func (s *fakeStore) Issue(_ context.Context, _ coretenant.Scope, id, number string, issueDate, dueDate time.Time, _ string) (domain.Invoice, error) {
	inv, ok := s.invoices[id]
	if !ok {
		return domain.Invoice{}, pgx.ErrNoRows
	}
	if inv.Status != domain.InvoiceDraft || s.issueRace {
		return domain.Invoice{}, repository.ErrInvoiceNotDraft
	}
	inv.Status, inv.InvoiceNumber, inv.IssueDate, inv.DueDate = domain.InvoiceIssued, number, &issueDate, &dueDate
	s.invoices[id] = inv
	return inv, nil
}

func (s *fakeStore) SetPDF(_ context.Context, _ coretenant.Scope, id, assetID string) error {
	inv, ok := s.invoices[id]
	if !ok {
		return pgx.ErrNoRows
	}
	inv.PDFAssetID = &assetID
	s.invoices[id] = inv
	return nil
}

func (s *fakeStore) Void(_ context.Context, _ coretenant.Scope, id, reason, _ string) (domain.Invoice, error) {
	inv, ok := s.invoices[id]
	if !ok {
		return domain.Invoice{}, pgx.ErrNoRows
	}
	if inv.AmountPaid != "0.00" && inv.AmountPaid != "" {
		return domain.Invoice{}, repository.ErrInvoiceHasPayments
	}
	if inv.Status == domain.InvoiceVoid || inv.Status == domain.InvoicePaid {
		return domain.Invoice{}, repository.ErrInvoiceNotPayable
	}
	inv.Status, inv.VoidReason = domain.InvoiceVoid, reason
	s.invoices[id] = inv
	return inv, nil
}

func (s *fakeStore) MarkOverdue(_ context.Context, _ coretenant.Scope, today time.Time) (int64, error) {
	s.overdueSeen = today
	return 1, nil
}

func (s *fakeStore) Record(_ context.Context, _ coretenant.Scope, invoiceID string, p repository.PaymentParams) (domain.Invoice, domain.Payment, bool, error) {
	inv, ok := s.invoices[invoiceID]
	if !ok {
		return domain.Invoice{}, domain.Payment{}, false, pgx.ErrNoRows
	}
	if p.Reference != "" && p.Method != "manual" {
		for _, pay := range s.payments {
			if pay.Method == p.Method && pay.Reference == p.Reference {
				return inv, pay, false, nil
			}
		}
	}
	if inv.Status != domain.InvoiceIssued && inv.Status != domain.InvoiceOverdue {
		return domain.Invoice{}, domain.Payment{}, false, repository.ErrInvoiceNotPayable
	}
	amount, _ := new(big.Rat).SetString(p.Amount)
	paid, _ := new(big.Rat).SetString(inv.AmountPaid)
	grand, _ := new(big.Rat).SetString(inv.GrandTotal)
	paid.Add(paid, amount)
	if paid.Cmp(grand) > 0 {
		return domain.Invoice{}, domain.Payment{}, false, repository.ErrPaymentExceeds
	}
	inv.AmountPaid = paid.FloatString(2)
	if paid.Cmp(grand) == 0 {
		inv.Status = domain.InvoicePaid
		inv.PaidAt = &p.PaidAt
	}
	s.invoices[invoiceID] = inv
	pay := domain.Payment{ID: fmt.Sprintf("pay-%d", len(s.payments)+1), InvoiceID: invoiceID, Amount: amount.FloatString(2), Method: p.Method, Reference: p.Reference, Note: p.Note, RecordedBy: p.RecordedBy, PaidAt: p.PaidAt}
	s.payments = append(s.payments, pay)
	return inv, pay, true, nil
}

type fakePayList struct{ *fakeStore }

func (f fakePayList) List(_ context.Context, _ coretenant.Scope, invoiceID string) ([]domain.Payment, error) {
	out := []domain.Payment{}
	for _, p := range f.payments {
		if p.InvoiceID == invoiceID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f fakePayList) ListAll(_ context.Context, _ coretenant.Scope, _, _ int) ([]domain.Payment, int64, error) {
	return f.payments, int64(len(f.payments)), nil
}

// fakePayments memisahkan List (invoice) dari store.List(filter) yang bentrok nama.
type fakePayments struct{ s *fakeStore }

func (f fakePayments) Record(c context.Context, sc coretenant.Scope, id string, p repository.PaymentParams) (domain.Invoice, domain.Payment, bool, error) {
	return f.s.Record(c, sc, id, p)
}
func (f fakePayments) List(c context.Context, sc coretenant.Scope, id string) ([]domain.Payment, error) {
	return fakePayList{f.s}.List(c, sc, id)
}
func (f fakePayments) ListAll(c context.Context, sc coretenant.Scope, l, o int) ([]domain.Payment, int64, error) {
	return fakePayList{f.s}.ListAll(c, sc, l, o)
}

// ---- fakes kecil ----

type fakeAccounts struct{ missing bool }

func (f fakeAccounts) Create(_ context.Context, _ coretenant.Scope, p repository.AccountParams, _ string) (domain.Account, error) {
	return domain.Account{ID: "acc-new", Name: p.Name, SourceType: p.SourceType, SourceID: p.SourceID}, nil
}
func (f fakeAccounts) Update(_ context.Context, _ coretenant.Scope, id string, p repository.AccountParams) (domain.Account, error) {
	return domain.Account{ID: id, Name: p.Name}, nil
}
func (f fakeAccounts) FindByID(_ context.Context, _ coretenant.Scope, id string) (domain.Account, error) {
	if f.missing {
		return domain.Account{}, pgx.ErrNoRows
	}
	return domain.Account{ID: id, Name: "Budi", Email: "budi@example.com"}, nil
}
func (f fakeAccounts) FindBySource(context.Context, coretenant.Scope, string, string) (domain.Account, error) {
	return domain.Account{}, pgx.ErrNoRows
}
func (f fakeAccounts) List(context.Context, coretenant.Scope, string, int, int) ([]domain.Account, int64, error) {
	return nil, 0, nil
}

type fakeCounter struct{ calls int }

func (c *fakeCounter) Next(context.Context, coretenant.Scope, string, int) (int, error) {
	c.calls++
	return c.calls, nil
}

type fakeSettings struct{ s domain.Settings }

func (f *fakeSettings) Get(context.Context, coretenant.Scope) (domain.Settings, error) {
	return f.s, nil
}
func (f *fakeSettings) Upsert(_ context.Context, _ coretenant.Scope, s domain.Settings, _ string) (domain.Settings, error) {
	f.s = s
	return s, nil
}

type fakeFiles struct {
	uploads, deletes int
	objects          map[string][]byte
	err              error
}

func (f *fakeFiles) UploadObject(_ context.Context, _ coretenant.Scope, _ assetservice.UploadObjectParams, content io.Reader) (assetdomain.AssetObject, error) {
	if f.err != nil {
		return assetdomain.AssetObject{}, f.err
	}
	f.uploads++
	b, _ := io.ReadAll(content)
	id := fmt.Sprintf("asset-%d", f.uploads)
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	f.objects[id] = b
	return assetdomain.AssetObject{ID: id}, nil
}
func (f *fakeFiles) OpenObject(_ context.Context, _ coretenant.Scope, id string) (io.ReadCloser, assetdomain.AssetObject, error) {
	b, ok := f.objects[id]
	if !ok {
		return nil, assetdomain.AssetObject{}, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), assetdomain.AssetObject{ID: id}, nil
}
func (f *fakeFiles) DeleteObject(_ context.Context, _ coretenant.Scope, id string) error {
	f.deletes++
	delete(f.objects, id)
	return nil
}

type fakeLinks struct {
	ensured, revoked int
	lastExpiry       time.Time
}

func (l *fakeLinks) Ensure(_ context.Context, _ coretenant.Scope, docType, docID string, expiresAt time.Time, _ string) (publiclink.Link, error) {
	l.ensured++
	l.lastExpiry = expiresAt
	return publiclink.Link{DocumentType: docType, DocumentID: docID, Token: "tok", ExpiresAt: expiresAt}, nil
}
func (l *fakeLinks) Revoke(context.Context, coretenant.Scope, string, string) error {
	l.revoked++
	return nil
}

type fakeSender struct {
	autoCalls int
	last      domain.Invoice
}

func (s *fakeSender) AutoSend(_ context.Context, _ coretenant.Scope, inv domain.Invoice, _ string) {
	s.autoCalls++
	s.last = inv
}

type fakeIssuer struct{}

func (fakeIssuer) OrganizationProfile(context.Context, coretenant.Scope) (docpdf.Party, error) {
	return docpdf.Party{Name: "PT Zyad Technovation"}, nil
}

type fakeRenderer struct{ docs []docpdf.Document }

func (r *fakeRenderer) Render(d docpdf.Document) ([]byte, error) {
	r.docs = append(r.docs, d)
	return []byte("%PDF-fake " + d.Stamp), nil
}

type recordingListener struct {
	paid      []InvoiceRef
	contracts []ContractRef
}

func (l *recordingListener) InvoicePaid(_ context.Context, _ coretenant.Scope, inv InvoiceRef) {
	l.paid = append(l.paid, inv)
}
func (l *recordingListener) ContractCreated(_ context.Context, _ coretenant.Scope, c ContractRef) {
	l.contracts = append(l.contracts, c)
}

type invoiceHarness struct {
	svc      *invoiceService
	pay      PaymentService
	store    *fakeStore
	counter  *fakeCounter
	files    *fakeFiles
	links    *fakeLinks
	sender   *fakeSender
	renderer *fakeRenderer
	listener *recordingListener
}

func newInvoiceHarness(t *testing.T) *invoiceHarness {
	t.Helper()
	h := &invoiceHarness{
		store: newFakeStore(), counter: &fakeCounter{}, files: &fakeFiles{}, links: &fakeLinks{},
		sender: &fakeSender{}, renderer: &fakeRenderer{}, listener: &recordingListener{},
	}
	registry := NewRegistry()
	registry.Add(h.listener)
	h.svc = NewInvoiceService(InvoiceDeps{
		Accounts: fakeAccounts{}, Invoices: h.store, Counters: h.counter,
		Settings: &fakeSettings{domain.Settings{InvoiceLeadDays: 7, PaymentTermsDays: 7, DefaultChannels: []string{"email"}}},
		Issuers:  fakeIssuer{}, Files: h.files, Renderer: h.renderer, Links: h.links, Sender: h.sender,
		Listeners: registry, FrontendURL: "https://app.test/", Now: func() time.Time { return nowWIB },
	}).(*invoiceService)
	h.pay = NewPaymentService(PaymentDeps{Payments: fakePayments{h.store}, Invoices: h.svc, Listeners: registry, Now: func() time.Time { return nowWIB }})
	return h
}

func line(desc, price string) LineInput {
	return LineInput{LineInput: pricing.LineInput{Description: desc, UnitPrice: price}}
}

func (h *invoiceHarness) draft(t *testing.T, in InvoiceInput) domain.Invoice {
	t.Helper()
	if in.AccountID == "" {
		in.AccountID = "a1"
	}
	if len(in.Lines) == 0 {
		in.Lines = []LineInput{line("Instalasi", "500000")}
	}
	inv, err := h.svc.CreateDraft(ctx, scope, in, "u1")
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	return inv
}

func wib() *time.Location { return businesstime.Location() }
