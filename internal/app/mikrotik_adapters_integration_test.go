//go:build integration

package app

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/core/businesstime"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/platform/database/testutil"
)

type usageFixture struct {
	ctx      context.Context
	tx       func(org string, fn func(tx pgx.Tx))
	reader   mikrotikReceivableReader
	tenants  testutil.TenantPair
	today    time.Time
	prodP    string
	prodQ    string
	accounts map[string]string
}

func setupUsageFixture(t *testing.T) usageFixture {
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
	tx := func(org string, fn func(tx pgx.Tx)) {
		t.Helper()
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", org); err != nil {
			t.Fatalf("scope: %v", err)
		}
		fn(tx)
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}
	f := usageFixture{
		ctx: ctx, tx: tx, tenants: tenants, accounts: map[string]string{},
		reader: mikrotikReceivableReader{db: db, now: time.Now},
		today:  businesstime.DayOf(time.Now()),
	}
	if err := db.QueryRow(ctx, `SELECT gen_random_uuid()::text, gen_random_uuid()::text`).Scan(&f.prodP, &f.prodQ); err != nil {
		t.Fatal(err)
	}
	cleanup := func() {
		for _, org := range []string{tenants.A.OrganizationID, tenants.B.OrganizationID} {
			tx, err := db.Begin(ctx)
			if err != nil {
				continue
			}
			_, _ = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", org)
			_, _ = tx.Exec(ctx, `DELETE FROM receivable_invoices WHERE organization_id = $1`, org)
			_, _ = tx.Exec(ctx, `DELETE FROM receivable_contracts WHERE organization_id = $1`, org)
			_, _ = tx.Exec(ctx, `DELETE FROM receivable_accounts WHERE organization_id = $1`, org)
			_ = tx.Commit(ctx)
		}
	}
	cleanup()
	t.Cleanup(cleanup)
	return f
}

func (f usageFixture) account(t *testing.T, org, name string) string {
	t.Helper()
	var id string
	f.tx(org, func(tx pgx.Tx) {
		if err := tx.QueryRow(f.ctx, `INSERT INTO receivable_accounts (organization_id, name) VALUES ($1, $2) RETURNING id::text`, org, name).Scan(&id); err != nil {
			t.Fatalf("account: %v", err)
		}
	})
	return id
}

func (f usageFixture) contract(t *testing.T, org, account, status string, products ...string) {
	t.Helper()
	f.tx(org, func(tx pgx.Tx) {
		var id string
		if err := tx.QueryRow(f.ctx, `
			INSERT INTO receivable_contracts (organization_id, account_id, contract_number, status, start_date)
			VALUES ($1, $2, 'C-'||gen_random_uuid()::text, $3, current_date) RETURNING id::text`, org, account, status).Scan(&id); err != nil {
			t.Fatalf("contract: %v", err)
		}
		for _, p := range products {
			if _, err := tx.Exec(f.ctx, `
				INSERT INTO receivable_contract_items (organization_id, contract_id, description, product_id, billing_frequency, payment_timing, next_period_start, next_period_end)
				VALUES ($1, $2, 'item', $3::uuid, 'monthly', 'prepaid', current_date, current_date)`, org, id, p); err != nil {
				t.Fatalf("contract item: %v", err)
			}
		}
	})
}

func (f usageFixture) invoice(t *testing.T, org, account, status string, due time.Time, products ...string) {
	t.Helper()
	f.tx(org, func(tx pgx.Tx) {
		var id string
		if err := tx.QueryRow(f.ctx, `
			INSERT INTO receivable_invoices (organization_id, account_id, status, due_date)
			VALUES ($1, $2, $3, $4::date) RETURNING id::text`, org, account, status, due).Scan(&id); err != nil {
			t.Fatalf("invoice: %v", err)
		}
		for _, p := range products {
			if _, err := tx.Exec(f.ctx, `
				INSERT INTO receivable_invoice_items (organization_id, invoice_id, description, product_id)
				VALUES ($1, $2, 'line', $3::uuid)`, org, id, p); err != nil {
				t.Fatalf("invoice item: %v", err)
			}
		}
	})
}

func scopeOf(t *testing.T, f usageFixture, org string) coretenant.Scope {
	if org == f.tenants.A.OrganizationID {
		return f.tenants.A.Scope
	}
	return f.tenants.B.Scope
}

func TestMikrotikReceivableReader_ActiveContractProductIDsIntegration(t *testing.T) {
	f := setupUsageFixture(t)
	a, b := f.tenants.A.OrganizationID, f.tenants.B.OrganizationID
	a1, a2, b1 := f.account(t, a, "a1"), f.account(t, a, "a2"), f.account(t, b, "b1")
	f.contract(t, a, a1, "active", f.prodP)
	f.contract(t, a, a2, "ended", f.prodQ) // ended contract: not in use
	f.contract(t, b, b1, "active", f.prodQ)

	got, err := f.reader.ActiveContractProductIDs(f.ctx, scopeOf(t, f, a))
	if err != nil || len(got) != 1 || got[0] != f.prodP {
		t.Fatalf("org A: %v %v (want only P)", got, err)
	}
	got, err = f.reader.ActiveContractProductIDs(f.ctx, scopeOf(t, f, b))
	sort.Strings(got)
	if err != nil || len(got) != 1 || got[0] != f.prodQ {
		t.Fatalf("org B: %v %v (want only Q)", got, err)
	}
}

func TestMikrotikReceivableReader_ForProductIntegration(t *testing.T) {
	f := setupUsageFixture(t)
	a, b := f.tenants.A.OrganizationID, f.tenants.B.OrganizationID
	a1, a2, b1 := f.account(t, a, "a1"), f.account(t, a, "a2"), f.account(t, b, "b1")

	// Active contracts holding P: two for a1 (one with P twice), none from the ended one or Q-only one.
	f.contract(t, a, a1, "active", f.prodP)
	f.contract(t, a, a1, "active", f.prodP, f.prodP)
	f.contract(t, a, a2, "active", f.prodQ)
	f.contract(t, a, a2, "ended", f.prodP)
	f.contract(t, b, b1, "active", f.prodP)

	day := func(n int) time.Time { return f.today.AddDate(0, 0, n) }
	f.invoice(t, a, a1, "issued", day(-1), f.prodP)           // counted
	f.invoice(t, a, a1, "overdue", day(-3), f.prodP, f.prodP) // counted once
	f.invoice(t, a, a1, "overdue", day(-10), f.prodP)         // before effectiveFrom
	f.invoice(t, a, a1, "paid", day(-2), f.prodP)             // not open
	f.invoice(t, a, a1, "draft", day(-2), f.prodP)            // not open
	f.invoice(t, a, a1, "issued", day(0), f.prodP)            // due today: not yet overdue
	f.invoice(t, a, a1, "issued", day(-1), f.prodQ)           // other product
	f.invoice(t, b, b1, "issued", day(-1), f.prodP)           // other organization

	r := f.reader
	r.now = func() time.Time { return time.Now() }
	u, err := r.ForProduct(f.ctx, f.tenants.A.Scope, f.prodP, day(-5))
	if err != nil {
		t.Fatal(err)
	}
	if u.ActiveContracts != 2 || u.Accounts != 1 || u.OverdueInvoices != 2 {
		t.Fatalf("org A usage = %+v, want contracts=2 accounts=1 overdue=2", u)
	}
	// effectiveFrom further back also includes the 10-day-old invoice.
	u, _ = r.ForProduct(f.ctx, f.tenants.A.Scope, f.prodP, day(-30))
	if u.OverdueInvoices != 3 {
		t.Fatalf("overdue from -30d = %d, want 3", u.OverdueInvoices)
	}
	u, _ = r.ForProduct(f.ctx, f.tenants.B.Scope, f.prodP, day(-5))
	if u.ActiveContracts != 1 || u.Accounts != 1 || u.OverdueInvoices != 1 {
		t.Fatalf("org B usage = %+v", u)
	}
	// Q for org B: nothing.
	u, _ = r.ForProduct(f.ctx, f.tenants.B.Scope, f.prodQ, day(-5))
	if u.ActiveContracts+u.Accounts+u.OverdueInvoices != 0 {
		t.Fatalf("org B must not see org A product usage: %+v", u)
	}
}
