//go:build integration

package repository_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
	"zyad.cloud/internal/shared/pricing"
)

type env struct {
	ctx      context.Context
	db       *database.Pool
	tenants  testutil.TenantPair
	accounts repository.AccountRepository
	invoices repository.InvoiceRepository
	payments repository.PaymentRepository
	sends    repository.SendRepository
	settings repository.SettingsRepository
	counters repository.CounterRepository
}

func setup(t *testing.T) env {
	t.Helper()
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	if _, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', 'organization-a', 'Organization A', 'active'),
		       ($2, 'customer', 'organization-b', 'Organization B', 'active')
		ON CONFLICT DO NOTHING`, tenants.A.OrganizationID, tenants.B.OrganizationID); err != nil {
		t.Fatalf("insert organizations: %v", err)
	}
	cleanup := func() {
		// receivable_* di bawah FORCE RLS: hapus per scope organisasi, anak dulu.
		for _, orgID := range []string{tenants.A.OrganizationID, tenants.B.OrganizationID} {
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Errorf("cleanup begin: %v", err)
				return
			}
			_, _ = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID)
			for _, table := range []string{
				"receivable_payments", "receivable_invoice_sends", "receivable_invoice_items", "receivable_invoices",
				"receivable_accounts", "receivable_document_counters", "receivable_settings",
			} {
				_, _ = tx.Exec(ctx, "DELETE FROM "+table+" WHERE organization_id = $1", orgID)
			}
			_ = tx.Commit(ctx)
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	return env{
		ctx: ctx, db: db, tenants: tenants,
		accounts: repository.NewAccountRepository(db), invoices: repository.NewInvoiceRepository(db),
		payments: repository.NewPaymentRepository(db), sends: repository.NewSendRepository(db),
		settings: repository.NewSettingsRepository(db), counters: repository.NewCounterRepository(db),
	}
}

func (e env) account(t *testing.T, scope coretenant.Scope, name string) domain.Account {
	t.Helper()
	a, err := e.accounts.Create(e.ctx, scope, repository.AccountParams{Name: name, Email: "budi@example.com"}, "")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	return a
}

func date(s string) *time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return &d
}

// params membangun invoice dari baris yang dihitung pricing.PriceLines (kode nyata).
func params(t *testing.T, accountID string, lines ...pricing.LineInput) repository.CreateInvoiceParams {
	t.Helper()
	totals, priced, err := pricing.PriceLines(lines)
	if err != nil {
		t.Fatalf("price lines: %v", err)
	}
	items := make([]repository.InvoiceItemParams, len(priced))
	for i, l := range priced {
		items[i] = repository.InvoiceItemParams{Line: l}
	}
	return repository.CreateInvoiceParams{AccountID: accountID, Currency: "IDR", Totals: totals, Items: items, Channels: []string{"email"}}
}

// threeThirtyThree: 33.000 sekali bayar + 300.000 bulanan prabayar = 333.000.
func threeThirtyThree(t *testing.T, accountID string) repository.CreateInvoiceParams {
	p := params(t, accountID,
		pricing.LineInput{Description: "Instalasi", UnitPrice: "33000"},
		pricing.LineInput{Description: "Internet 50 Mbps", UnitPrice: "300000", Attrs: pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid}},
	)
	p.Items[1].PeriodStart, p.Items[1].PeriodEnd = date("2026-10-01"), date("2026-10-31")
	return p
}

func (e env) issued(t *testing.T, scope coretenant.Scope, accountID, number string) domain.Invoice {
	t.Helper()
	inv, err := e.invoices.Create(e.ctx, scope, threeThirtyThree(t, accountID))
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	inv, err = e.invoices.Issue(e.ctx, scope, inv.ID, number, *date("2026-10-05"), *date("2026-10-12"), "")
	if err != nil {
		t.Fatalf("issue invoice: %v", err)
	}
	return inv
}

func TestInvoiceLifecycle(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	acc := e.account(t, scope, "Budi")

	draft, err := e.invoices.Create(e.ctx, scope, threeThirtyThree(t, acc.ID))
	if err != nil {
		t.Fatal(err)
	}
	if draft.Status != domain.InvoiceDraft || draft.InvoiceNumber != "" || draft.GrandTotal != "333000.00" || len(draft.Items) != 2 || draft.Account.Name != "Budi" {
		t.Fatalf("draft = %+v", draft)
	}
	if draft.Items[1].Pricing.Frequency != pricing.Monthly || draft.Items[1].PeriodStart == nil {
		t.Fatalf("item 2 = %+v", draft.Items[1])
	}

	inv, err := e.invoices.Issue(e.ctx, scope, draft.ID, "INV-2026-0001", *date("2026-10-05"), *date("2026-10-12"), "")
	if err != nil || inv.Status != domain.InvoiceIssued || inv.InvoiceNumber != "INV-2026-0001" || inv.DueDate.Format("2006-01-02") != "2026-10-12" {
		t.Fatalf("issued = %+v err=%v", inv, err)
	}
	if _, err := e.invoices.Issue(e.ctx, scope, draft.ID, "INV-2026-0002", *date("2026-10-05"), *date("2026-10-12"), ""); !errors.Is(err, repository.ErrInvoiceNotDraft) {
		t.Fatalf("second issue err = %v", err)
	}

	inv, _, created, err := e.payments.Record(e.ctx, scope, draft.ID, repository.PaymentParams{Amount: "100000", Method: "manual", PaidAt: time.Now().UTC()})
	if err != nil || !created || inv.Status != domain.InvoiceIssued || inv.AmountPaid != "100000.00" || inv.Balance() != "233000.00" {
		t.Fatalf("partial = %+v created=%v err=%v", inv, created, err)
	}
	if _, _, _, err := e.payments.Record(e.ctx, scope, draft.ID, repository.PaymentParams{Amount: "300000", Method: "manual", PaidAt: time.Now().UTC()}); !errors.Is(err, repository.ErrPaymentExceeds) {
		t.Fatalf("overpay err = %v", err)
	}
	inv, _, _, err = e.payments.Record(e.ctx, scope, draft.ID, repository.PaymentParams{Amount: "233000", Method: "manual", PaidAt: time.Now().UTC()})
	if err != nil || inv.Status != domain.InvoicePaid || inv.PaidAt == nil || inv.Balance() != "0.00" {
		t.Fatalf("paid = %+v err=%v", inv, err)
	}
	if _, err := e.invoices.Void(e.ctx, scope, draft.ID, "salah", ""); !errors.Is(err, repository.ErrInvoiceHasPayments) {
		t.Fatalf("void paid err = %v", err)
	}
	if _, _, _, err := e.payments.Record(e.ctx, scope, draft.ID, repository.PaymentParams{Amount: "1", Method: "manual", PaidAt: time.Now().UTC()}); !errors.Is(err, repository.ErrInvoiceNotPayable) {
		t.Fatalf("pay paid err = %v", err)
	}
	list, err := e.payments.List(e.ctx, scope, draft.ID)
	if err != nil || len(list) != 2 {
		t.Fatalf("payments = %+v err=%v", list, err)
	}
	all, total, err := e.payments.ListAll(e.ctx, scope, 10, 0)
	if err != nil || total != 2 || len(all) != 2 || all[0].InvoiceNumber != "INV-2026-0001" || all[0].AccountName != "Budi" {
		t.Fatalf("all = %+v total=%d err=%v", all, total, err)
	}
}

func TestVoidDraftAndIssued(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	acc := e.account(t, scope, "Budi")
	draft, _ := e.invoices.Create(e.ctx, scope, threeThirtyThree(t, acc.ID))
	v, err := e.invoices.Void(e.ctx, scope, draft.ID, "dibatalkan", "")
	if err != nil || v.Status != domain.InvoiceVoid || v.VoidReason != "dibatalkan" || v.VoidedAt == nil {
		t.Fatalf("void = %+v err=%v", v, err)
	}
	if _, err := e.invoices.Void(e.ctx, scope, draft.ID, "lagi", ""); !errors.Is(err, repository.ErrInvoiceNotPayable) {
		t.Fatalf("void twice err = %v", err)
	}
	if _, err := e.invoices.ReplaceDraft(e.ctx, scope, draft.ID, threeThirtyThree(t, acc.ID)); !errors.Is(err, repository.ErrInvoiceNotDraft) {
		t.Fatalf("replace non-draft err = %v", err)
	}
}

func TestReplaceDraftSwapsItems(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	acc := e.account(t, scope, "Budi")
	draft, _ := e.invoices.Create(e.ctx, scope, threeThirtyThree(t, acc.ID))
	next := params(t, acc.ID, pricing.LineInput{Description: "Hanya satu", UnitPrice: "1000"})
	next.Notes = "revisi"
	got, err := e.invoices.ReplaceDraft(e.ctx, scope, draft.ID, next)
	if err != nil || len(got.Items) != 1 || got.GrandTotal != "1000.00" || got.Notes != "revisi" {
		t.Fatalf("replaced = %+v err=%v", got, err)
	}
}

func TestPaymentConcurrency(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	inv := e.issued(t, scope, e.account(t, scope, "Budi").ID, "INV-2026-0001")

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, _, errs[i] = e.payments.Record(e.ctx, scope, inv.ID, repository.PaymentParams{Amount: "200000", Method: "manual", PaidAt: time.Now().UTC()})
		}(i)
	}
	wg.Wait()
	ok, exceeded := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, repository.ErrPaymentExceeds):
			exceeded++
		default:
			t.Fatalf("unexpected err = %v", err)
		}
	}
	if ok != 1 || exceeded != 1 {
		t.Fatalf("ok=%d exceeded=%d, want 1/1", ok, exceeded)
	}
	got, _ := e.invoices.FindByID(e.ctx, scope, inv.ID)
	if got.AmountPaid != "200000.00" {
		t.Fatalf("amount paid = %s", got.AmountPaid)
	}
}

func TestDuplicateSourceAndContractPeriod(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	acc := e.account(t, scope, "Budi")
	so := "aaaaaaaa-0000-0000-0000-000000000001"

	p := threeThirtyThree(t, acc.ID)
	p.SourceType, p.SourceID, p.IdempotencyKey = "sales_order", so, "initial"
	first, err := e.invoices.Create(e.ctx, scope, p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.invoices.Create(e.ctx, scope, p); !errors.Is(err, repository.ErrDuplicateSource) {
		t.Fatalf("duplicate err = %v", err)
	}
	found, err := e.invoices.FindBySource(e.ctx, scope, "sales_order", so, "initial")
	if err != nil || found.ID != first.ID {
		t.Fatalf("FindBySource = %+v err=%v", found, err)
	}
	if _, err := e.invoices.FindBySource(e.ctx, scope, "sales_order", so, "nope"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing err = %v", err)
	}

	ci := "bbbbbbbb-0000-0000-0000-000000000001"
	withContractItem := func() repository.CreateInvoiceParams {
		c := params(t, acc.ID, pricing.LineInput{Description: "Internet", UnitPrice: "1000"})
		c.Items[0].ContractItemID, c.Items[0].PeriodStart, c.Items[0].PeriodEnd = ci, date("2026-11-01"), date("2026-11-30")
		return c
	}
	if _, err := e.invoices.Create(e.ctx, scope, withContractItem()); err != nil {
		t.Fatal(err)
	}
	if _, err := e.invoices.Create(e.ctx, scope, withContractItem()); err == nil {
		t.Fatal("same contract_item_id + period_start must be rejected")
	}
}

func TestPaymentReferenceIdempotent(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	inv := e.issued(t, scope, e.account(t, scope, "Budi").ID, "INV-2026-0001")
	pay := repository.PaymentParams{Amount: "100000", Method: "doku", Reference: "R1", PaidAt: time.Now().UTC(), ProviderPayload: map[string]any{"status": "SUCCESS"}}

	_, first, created, err := e.payments.Record(e.ctx, scope, inv.ID, pay)
	if err != nil || !created {
		t.Fatalf("first created=%v err=%v", created, err)
	}
	got, second, created, err := e.payments.Record(e.ctx, scope, inv.ID, pay)
	if err != nil || created || second.ID != first.ID || got.AmountPaid != "100000.00" {
		t.Fatalf("second = %+v created=%v paid=%s err=%v", second, created, got.AmountPaid, err)
	}
}

func TestMarkOverdue(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	acc := e.account(t, scope, "Budi")
	late, _ := e.invoices.Create(e.ctx, scope, threeThirtyThree(t, acc.ID))
	late, err := e.invoices.Issue(e.ctx, scope, late.ID, "INV-2026-0001", *date("2026-09-01"), *date("2026-09-08"), "")
	if err != nil {
		t.Fatal(err)
	}
	draft, _ := e.invoices.Create(e.ctx, scope, threeThirtyThree(t, acc.ID))
	notDue := e.issued(t, scope, acc.ID, "INV-2026-0002") // due 2026-10-12

	n, err := e.invoices.MarkOverdue(e.ctx, scope, *date("2026-10-06"))
	if err != nil || n != 1 {
		t.Fatalf("marked = %d err=%v", n, err)
	}
	for id, want := range map[string]domain.InvoiceStatus{late.ID: domain.InvoiceOverdue, draft.ID: domain.InvoiceDraft, notDue.ID: domain.InvoiceIssued} {
		if got, _ := e.invoices.FindByID(e.ctx, scope, id); got.Status != want {
			t.Fatalf("invoice %s status = %s, want %s", id, got.Status, want)
		}
	}
	// invoice overdue masih bisa dibayar sampai lunas.
	if inv, _, _, err := e.payments.Record(e.ctx, scope, late.ID, repository.PaymentParams{Amount: "333000", Method: "manual", PaidAt: time.Now().UTC()}); err != nil || inv.Status != domain.InvoicePaid {
		t.Fatalf("pay overdue = %+v err=%v", inv, err)
	}
}

func TestListFilters(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	budi, siti := e.account(t, scope, "Budi"), e.account(t, scope, "Siti")
	inv := e.issued(t, scope, budi.ID, "INV-2026-0001")
	_, _ = e.invoices.Create(e.ctx, scope, threeThirtyThree(t, siti.ID))

	all, total, err := e.invoices.List(e.ctx, scope, repository.InvoiceListFilter{})
	if err != nil || total != 2 || len(all) != 2 {
		t.Fatalf("all = %d total=%d err=%v", len(all), total, err)
	}
	if got, total, _ := e.invoices.List(e.ctx, scope, repository.InvoiceListFilter{Status: "issued"}); total != 1 || got[0].ID != inv.ID {
		t.Fatalf("status filter = %+v", got)
	}
	if got, total, _ := e.invoices.List(e.ctx, scope, repository.InvoiceListFilter{Search: "siti"}); total != 1 || got[0].Account.Name != "Siti" {
		t.Fatalf("search by account = %+v", got)
	}
	if _, total, _ := e.invoices.List(e.ctx, scope, repository.InvoiceListFilter{Search: "INV-2026-0001"}); total != 1 {
		t.Fatalf("search by number total = %d", total)
	}
	if _, total, _ := e.invoices.List(e.ctx, scope, repository.InvoiceListFilter{AccountID: siti.ID}); total != 1 {
		t.Fatalf("account filter total = %d", total)
	}
	// SendFailed: gagal tanpa kiriman sukses sesudahnya pada kanal yang sama.
	if _, err := e.sends.Record(e.ctx, scope, repository.RecordSendParams{InvoiceID: inv.ID, Channel: "email", Status: "failed", Error: "Belum ada pengirim.", Trigger: "auto"}); err != nil {
		t.Fatal(err)
	}
	if got, total, _ := e.invoices.List(e.ctx, scope, repository.InvoiceListFilter{SendFailed: true}); total != 1 || got[0].ID != inv.ID {
		t.Fatalf("send_failed filter = %+v", got)
	}
	if _, err := e.sends.Record(e.ctx, scope, repository.RecordSendParams{InvoiceID: inv.ID, Channel: "email", Status: "sent", Trigger: "manual"}); err != nil {
		t.Fatal(err)
	}
	if _, total, _ := e.invoices.List(e.ctx, scope, repository.InvoiceListFilter{SendFailed: true}); total != 0 {
		t.Fatalf("send_failed after successful retry total = %d", total)
	}
}

func TestSendLog(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	inv := e.issued(t, scope, e.account(t, scope, "Budi").ID, "INV-2026-0001")

	first, err := e.sends.Record(e.ctx, scope, repository.RecordSendParams{InvoiceID: inv.ID, Channel: "email", Recipient: "budi@example.com", Status: "failed", Error: "Mailbox tidak aktif", ClientRequestID: "req-1", Trigger: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.sends.Record(e.ctx, scope, repository.RecordSendParams{InvoiceID: inv.ID, Channel: "email", Status: "sent", ClientRequestID: "req-1", Trigger: "manual"}); err == nil {
		t.Fatal("duplicate client_request_id must fail")
	}
	got, err := e.sends.FindByClientRequest(e.ctx, scope, inv.ID, "req-1")
	if err != nil || got.ID != first.ID || got.Error != "Mailbox tidak aktif" {
		t.Fatalf("found = %+v err=%v", got, err)
	}
	if _, err := e.sends.FindByClientRequest(e.ctx, scope, inv.ID, "nope"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing err = %v", err)
	}
	if lf, err := e.sends.LatestFailedByInvoice(e.ctx, scope, inv.ID); err != nil || lf.ID != first.ID {
		t.Fatalf("latest failed = %+v err=%v", lf, err)
	}
	_, _ = e.sends.Record(e.ctx, scope, repository.RecordSendParams{InvoiceID: inv.ID, Channel: "whatsapp", Recipient: "6281234567890", Status: "sent", ExternalMessageID: "true_628", Trigger: "auto"})
	list, err := e.sends.ListByInvoice(e.ctx, scope, inv.ID)
	if err != nil || len(list) != 2 || list[0].Channel != "whatsapp" {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	if other, _ := e.sends.ListByInvoice(e.ctx, e.tenants.B.Scope, inv.ID); len(other) != 0 {
		t.Fatal("tenant B must not see tenant A sends")
	}
}

func TestReceivableIsolation(t *testing.T) {
	e := setup(t)
	a, b := e.tenants.A.Scope, e.tenants.B.Scope
	acc := e.account(t, a, "Budi")
	inv := e.issued(t, a, acc.ID, "INV-2026-0001")
	_, _, _, _ = e.payments.Record(e.ctx, a, inv.ID, repository.PaymentParams{Amount: "1000", Method: "manual", PaidAt: time.Now().UTC()})

	if _, err := e.accounts.FindByID(e.ctx, b, acc.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("account leak err = %v", err)
	}
	if _, err := e.invoices.FindByID(e.ctx, b, inv.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("invoice leak err = %v", err)
	}
	if _, _, _, err := e.payments.Record(e.ctx, b, inv.ID, repository.PaymentParams{Amount: "1000", Method: "manual", PaidAt: time.Now().UTC()}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-tenant payment err = %v", err)
	}
	if list, total, _ := e.invoices.List(e.ctx, b, repository.InvoiceListFilter{}); total != 0 || len(list) != 0 {
		t.Fatalf("invoice list leak = %+v", list)
	}
	if list, total, _ := e.payments.ListAll(e.ctx, b, 10, 0); total != 0 || len(list) != 0 {
		t.Fatalf("payment list leak = %+v", list)
	}
	if list, total, _ := e.accounts.List(e.ctx, b, "", 10, 0); total != 0 || len(list) != 0 {
		t.Fatalf("account list leak = %+v", list)
	}
	na, _ := e.counters.Next(e.ctx, a, "invoice", 2026)
	nb, _ := e.counters.Next(e.ctx, b, "invoice", 2026)
	na2, _ := e.counters.Next(e.ctx, a, "invoice", 2026)
	if na != 1 || nb != 1 || na2 != 2 {
		t.Fatalf("counters a=%d b=%d a2=%d", na, nb, na2)
	}
}

func TestAccountSourceAndUpdate(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	contact := "cccccccc-0000-0000-0000-000000000001"
	acc, err := e.accounts.Create(e.ctx, scope, repository.AccountParams{Name: "Budi", SourceType: "crm_contact", SourceID: contact}, "")
	if err != nil || acc.ContactID() != contact {
		t.Fatalf("account = %+v err=%v", acc, err)
	}
	if _, err := e.accounts.Create(e.ctx, scope, repository.AccountParams{Name: "Dobel", SourceType: "crm_contact", SourceID: contact}, ""); !errors.Is(err, repository.ErrDuplicateAccountSource) {
		t.Fatalf("duplicate source err = %v", err)
	}
	by, err := e.accounts.FindBySource(e.ctx, scope, "crm_contact", contact)
	if err != nil || by.ID != acc.ID {
		t.Fatalf("FindBySource = %+v err=%v", by, err)
	}
	upd, err := e.accounts.Update(e.ctx, scope, acc.ID, repository.AccountParams{Name: "Budi S.", CompanyName: "PT Maju", Email: "b@x.id"})
	if err != nil || upd.Name != "Budi S." || upd.CompanyName != "PT Maju" || upd.SourceID != contact {
		t.Fatalf("updated = %+v err=%v", upd, err)
	}
	manual := e.account(t, scope, "Manual")
	if manual.ContactID() != "" {
		t.Fatalf("manual account must have no contact: %+v", manual)
	}
	if _, total, _ := e.accounts.List(e.ctx, scope, "maju", 10, 0); total != 1 {
		t.Fatalf("search total = %d", total)
	}
}

func TestSettingsDefault(t *testing.T) {
	e := setup(t)
	scope := e.tenants.A.Scope
	got, err := e.settings.Get(e.ctx, scope)
	if err != nil || got.InvoiceLeadDays != 7 || got.PaymentTermsDays != 7 || len(got.DefaultChannels) != 1 || got.DefaultChannels[0] != "email" || got.DefaultSenderUserID != "" {
		t.Fatalf("default = %+v err=%v", got, err)
	}
	saved, err := e.settings.Upsert(e.ctx, scope, domain.Settings{InvoiceLeadDays: 14, PaymentTermsDays: 30, DefaultChannels: []string{"email", "whatsapp"}}, "")
	if err != nil || saved.PaymentTermsDays != 30 || len(saved.DefaultChannels) != 2 {
		t.Fatalf("saved = %+v err=%v", saved, err)
	}
	if again, _ := e.settings.Get(e.ctx, scope); again.InvoiceLeadDays != 14 {
		t.Fatalf("persisted = %+v", again)
	}
	if _, err := e.settings.Upsert(e.ctx, scope, domain.Settings{InvoiceLeadDays: 61, PaymentTermsDays: 7, DefaultChannels: []string{"email"}}, ""); err == nil {
		t.Fatal("lead days > 60 must be rejected by the database CHECK")
	}
}

// Referensi pembayaran manual bebas diisi tenant (mis. nomor transfer): tidak boleh
// bentrok lintas tenant maupun antar-pembayaran, hanya referensi provider yang idempoten.
func TestManualPaymentReferenceNotGloballyUnique(t *testing.T) {
	e := setup(t)
	a, b := e.tenants.A.Scope, e.tenants.B.Scope
	invA := e.issued(t, a, e.account(t, a, "Budi").ID, "INV-2026-0001")
	invB := e.issued(t, b, e.account(t, b, "Siti").ID, "INV-2026-0001")
	pay := repository.PaymentParams{Amount: "1000", Method: "manual", Reference: "TRF-001", PaidAt: time.Now().UTC()}

	for _, tc := range []struct {
		scope coretenant.Scope
		id    string
	}{{a, invA.ID}, {b, invB.ID}, {a, invA.ID}} {
		if _, _, created, err := e.payments.Record(e.ctx, tc.scope, tc.id, pay); err != nil || !created {
			t.Fatalf("manual payment with shared reference: created=%v err=%v", created, err)
		}
	}
	if got, _ := e.invoices.FindByID(e.ctx, a, invA.ID); got.AmountPaid != "2000.00" {
		t.Fatalf("both manual payments on A must count, paid = %s", got.AmountPaid)
	}
}
