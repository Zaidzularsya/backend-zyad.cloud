//go:build integration

package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	assetdomain "zyad.cloud/internal/modules/asset/domain"
	assetservice "zyad.cloud/internal/modules/asset/service"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/modules/receivable/repository"
	"zyad.cloud/internal/platform/database/testutil"
	"zyad.cloud/internal/shared/pricing"
)

// lockedFiles: FileStore dalam memori yang aman untuk goroutine paralel dengan id UUID
// (kolom pdf_asset_id bertipe uuid).
type lockedFiles struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (f *lockedFiles) UploadObject(_ context.Context, _ coretenant.Scope, _ assetservice.UploadObjectParams, content io.Reader) (assetdomain.AssetObject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, _ := io.ReadAll(content)
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	id := fmt.Sprintf("00000000-0000-4000-8000-%012d", len(f.objects)+1)
	f.objects[id] = b
	return assetdomain.AssetObject{ID: id}, nil
}

func (f *lockedFiles) OpenObject(_ context.Context, _ coretenant.Scope, id string) (io.ReadCloser, assetdomain.AssetObject, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.objects[id]
	if !ok {
		return nil, assetdomain.AssetObject{}, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(b)), assetdomain.AssetObject{ID: id}, nil
}

func (f *lockedFiles) DeleteObject(_ context.Context, _ coretenant.Scope, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, id)
	return nil
}

// TestBillingRunParallelWorkersNoDuplicateInvoices: dua worker berjalan bersamaan pada DB nyata
// untuk contract bulanan yang tertinggal 4 periode → tepat 4 invoice, bukan 8.
func TestBillingRunParallelWorkersNoDuplicateInvoices(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	bg := context.Background()
	tenants := testutil.NewTenantPair(t)
	sc := tenants.A.Scope
	if _, err := db.Exec(bg, `INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', 'organization-a', 'Organization A', 'active') ON CONFLICT DO NOTHING`, tenants.A.OrganizationID); err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	cleanup := func() {
		tx, err := db.Begin(bg)
		if err != nil {
			t.Errorf("cleanup begin: %v", err)
			return
		}
		_, _ = tx.Exec(bg, "SELECT set_config('app.organization_id', $1, true)", tenants.A.OrganizationID)
		for _, table := range []string{"receivable_payments", "receivable_invoice_sends", "receivable_invoice_items", "receivable_invoices",
			"receivable_contract_items", "receivable_contracts", "receivable_accounts", "receivable_document_counters", "receivable_settings"} {
			_, _ = tx.Exec(bg, "DELETE FROM "+table+" WHERE organization_id = $1", tenants.A.OrganizationID)
		}
		_ = tx.Commit(bg)
	}
	cleanup()
	t.Cleanup(cleanup)

	accounts, invoices, contracts := repository.NewAccountRepository(db), repository.NewInvoiceRepository(db), repository.NewContractRepository(db)
	settings := repository.NewSettingsRepository(db)
	acc, err := accounts.Create(bg, sc, repository.AccountParams{Name: "Budi", Email: "budi@example.com"}, "")
	if err != nil {
		t.Fatal(err)
	}
	start := date("2026-10-05")
	from, to := pricing.PeriodRange(start, pricing.Monthly, 1)
	created, err := contracts.Create(bg, sc, repository.CreateContractParams{
		Number: "CTR-RUN-1", AccountID: acc.ID, SourceType: "manual", StartDate: start, Channels: []string{"email"},
		Items: []repository.ContractItemParams{{Description: "Internet", Quantity: "1", UnitPrice: "300000", TaxPercent: "0",
			Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid, PeriodIndex: 1, NextPeriodStart: from, NextPeriodEnd: to}},
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2027, 1, 30, 3, 0, 0, 0, time.UTC)
	svc := NewInvoiceService(InvoiceDeps{
		Accounts: accounts, Invoices: invoices, Counters: repository.NewCounterRepository(db), Settings: settings,
		Issuers: fakeIssuer{}, Files: &lockedFiles{}, Renderer: &fakeRenderer{}, Links: &fakeLinks{}, Sender: &fakeSender{},
		Listeners: NewRegistry(), FrontendURL: "https://app.test/", Now: func() time.Time { return now },
	})
	run := NewBillingRun(contracts, svc, settings, NewRegistry(), func() time.Time { return now })

	var wg sync.WaitGroup
	results := make([]BillingRunResult, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := run.Run(bg, sc)
			if err != nil {
				t.Errorf("worker %d: %v", i, err)
			}
			results[i] = res
		}()
	}
	wg.Wait()

	// tabel di bawah FORCE RLS: hitung di dalam transaksi ber-scope organisasi.
	var count int
	tx, err := db.Begin(bg)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(bg)
	if _, err := tx.Exec(bg, "SELECT set_config('app.organization_id', $1, true)", tenants.A.OrganizationID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(bg, `SELECT count(*) FROM receivable_invoices WHERE organization_id = $1 AND contract_id = $2`,
		tenants.A.OrganizationID, created.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("invoices = %d, want 4 (results=%+v)", count, results)
	}
	after, _ := contracts.FindByID(bg, sc, created.ID)
	if after.Items[0].PeriodIndex != 5 || after.Items[0].NextPeriodStart.Format("2006-01-02") != "2027-03-05" {
		t.Fatalf("item after run = %+v", after.Items[0])
	}
	if st := after.Status; st != domain.ContractActive {
		t.Fatalf("status = %s", st)
	}
}
