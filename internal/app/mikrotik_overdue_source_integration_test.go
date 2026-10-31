//go:build integration

package app

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/mikrotik"
)

func (f usageFixture) trigger(t *testing.T, org, product string, enabled bool, from time.Time) {
	t.Helper()
	f.tx(org, func(tx pgx.Tx) {
		if _, err := tx.Exec(f.ctx, `
			INSERT INTO mikrotik_product_triggers (organization_id, product_id, enabled, effective_from)
			VALUES ($1, $2::uuid, $3, $4::date)
			ON CONFLICT (organization_id, product_id) DO UPDATE SET enabled = EXCLUDED.enabled, effective_from = EXCLUDED.effective_from`,
			org, product, enabled, from); err != nil {
			t.Fatalf("trigger: %v", err)
		}
	})
}

// invoiceNo inserts an invoice with a number, totals and a mix of items
// (empty string = item without product_id), returning its id.
func (f usageFixture) invoiceNo(t *testing.T, org, account, status, number string, due time.Time, paid string, products ...string) string {
	t.Helper()
	var id string
	f.tx(org, func(tx pgx.Tx) {
		if err := tx.QueryRow(f.ctx, `
			INSERT INTO receivable_invoices (organization_id, account_id, invoice_number, status, due_date, grand_total, amount_paid)
			VALUES ($1, $2, $3, $4, $5::date, 150000, $6::numeric) RETURNING id::text`,
			org, account, number, status, due, paid).Scan(&id); err != nil {
			t.Fatalf("invoice: %v", err)
		}
		for _, p := range products {
			var pid interface{}
			if p != "" {
				pid = p
			}
			if _, err := tx.Exec(f.ctx, `
				INSERT INTO receivable_invoice_items (organization_id, invoice_id, description, product_id)
				VALUES ($1, $2, 'line', $3::uuid)`, org, id, pid); err != nil {
				t.Fatalf("invoice item: %v", err)
			}
		}
	})
	return id
}

func TestTriggerInvoices_Matrix(t *testing.T) {
	f := setupUsageFixture(t)
	a, b := f.tenants.A.OrganizationID, f.tenants.B.OrganizationID
	sa, sb := scopeOf(t, f, a), scopeOf(t, f, b)
	acc, other, accB := f.account(t, a, "acc"), f.account(t, a, "other"), f.account(t, b, "accb")

	var disabledProd string
	if err := f.reader.db.QueryRow(f.ctx, `SELECT gen_random_uuid()::text`).Scan(&disabledProd); err != nil {
		t.Fatal(err)
	}
	day := func(n int) time.Time { return f.today.AddDate(0, 0, n) }
	f.trigger(t, a, f.prodP, true, day(-30))
	f.trigger(t, a, disabledProd, false, day(-30))
	f.trigger(t, b, f.prodP, true, day(-30))

	cases := []struct {
		name, status string
		due          time.Time
		products     []string
		want         bool
	}{
		{"due today", "issued", day(0), []string{f.prodP}, false},
		{"due yesterday", "issued", day(-1), []string{f.prodP}, true},
		{"overdue status", "overdue", day(-2), []string{f.prodP}, true},
		{"before effective_from", "issued", day(-31), []string{f.prodP}, false},
		{"on effective_from", "issued", day(-30), []string{f.prodP}, true},
		{"draft", "draft", day(-3), []string{f.prodP}, false},
		{"void", "void", day(-3), []string{f.prodP}, false},
		{"paid", "paid", day(-3), []string{f.prodP}, false},
		{"disabled product", "issued", day(-3), []string{disabledProd}, false},
		{"item without product", "issued", day(-3), []string{""}, false},
		{"unmarked product", "issued", day(-3), []string{f.prodQ}, false},
		{"mixed invoice", "issued", day(-3), []string{"", f.prodQ, f.prodP}, true},
	}
	want := map[string]bool{}
	for i, c := range cases {
		num := "T-" + string(rune('A'+i))
		id := f.invoiceNo(t, a, acc, c.status, num, c.due, "0", c.products...)
		want[id] = c.want
	}
	// Partially paid stays a candidate; balance reflects the remainder.
	partial := f.invoiceNo(t, a, acc, "issued", "T-PARTIAL", day(-4), "50000", f.prodP)
	want[partial] = true
	// Another account in the same org must not leak.
	f.invoiceNo(t, a, other, "issued", "T-OTHER", day(-5), "0", f.prodP)
	// Another organization's invoice must not leak.
	f.invoiceNo(t, b, accB, "issued", "T-ORGB", day(-5), "0", f.prodP)

	got, err := f.reader.TriggerInvoices(f.ctx, sa, acc, f.today)
	if err != nil {
		t.Fatal(err)
	}
	gotSet := map[string]bool{}
	for _, inv := range got {
		gotSet[inv.ID] = true
		if inv.ID == partial && (inv.Balance != "100000.00" || inv.Number != "T-PARTIAL") {
			t.Fatalf("partial invoice: %+v", inv)
		}
	}
	for id, w := range want {
		if gotSet[id] != w {
			t.Fatalf("invoice %s: got candidate=%v want %v", id, gotSet[id], w)
		}
	}
	for i := 1; i < len(got); i++ {
		if got[i].DueDate.Before(got[i-1].DueDate) {
			t.Fatalf("not ordered by due_date: %+v", got)
		}
	}

	// Organization scoping: org B scope sees nothing for org A's account.
	if gb, err := f.reader.TriggerInvoices(f.ctx, sb, acc, f.today); err != nil || len(gb) != 0 {
		t.Fatalf("org B on org A account: %v %v", gb, err)
	}
	// Org A's account scoped: other account's invoice excluded.
	if len(got) == 0 {
		t.Fatal("expected candidates")
	}
	for _, inv := range got {
		if inv.Number == "T-OTHER" || inv.Number == "T-ORGB" {
			t.Fatalf("leaked invoice %+v", inv)
		}
	}
}

func TestTriggerInvoices_AccountsKeysetPaging(t *testing.T) {
	f := setupUsageFixture(t)
	a := f.tenants.A.OrganizationID
	sa := scopeOf(t, f, a)
	f.trigger(t, a, f.prodP, true, f.today.AddDate(0, 0, -30))
	var ids []string
	for i := 0; i < 5; i++ {
		acc := f.account(t, a, "p"+string(rune('0'+i)))
		ids = append(ids, acc)
		// two invoices on the first account: it must still be listed once.
		f.invoiceNo(t, a, acc, "issued", "P-"+string(rune('0'+i)), f.today.AddDate(0, 0, -2), "0", f.prodP)
		if i == 0 {
			f.invoiceNo(t, a, acc, "overdue", "P-X", f.today.AddDate(0, 0, -3), "0", f.prodP)
		}
	}
	// an account with only a non-candidate invoice
	noise := f.account(t, a, "noise")
	f.invoiceNo(t, a, noise, "paid", "P-NOISE", f.today.AddDate(0, 0, -2), "0", f.prodP)

	var walked []string
	after := ""
	for guard := 0; guard < 10; guard++ {
		page, err := f.reader.AccountsWithTriggerInvoices(f.ctx, sa, f.today, after, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			break
		}
		walked = append(walked, page...)
		after = page[len(page)-1]
	}
	if len(walked) != 5 {
		t.Fatalf("walked %v", walked)
	}
	for i := 1; i < len(walked); i++ {
		if walked[i] <= walked[i-1] {
			t.Fatalf("not ascending/unique: %v", walked)
		}
	}
	set := map[string]bool{}
	for _, w := range walked {
		set[w] = true
	}
	for _, id := range ids {
		if !set[id] {
			t.Fatalf("missing %s in %v", id, walked)
		}
	}
	if set[noise] {
		t.Fatal("non-candidate account listed")
	}
}

func TestTriggerInvoices_AccountOfInvoice(t *testing.T) {
	f := setupUsageFixture(t)
	a, b := f.tenants.A.OrganizationID, f.tenants.B.OrganizationID
	acc, accB := f.account(t, a, "acc"), f.account(t, b, "accb")
	inv := f.invoiceNo(t, a, acc, "issued", "AO-1", f.today.AddDate(0, 0, -1), "0")
	invB := f.invoiceNo(t, b, accB, "issued", "AO-2", f.today.AddDate(0, 0, -1), "0")

	got, err := f.reader.AccountOfInvoice(f.ctx, scopeOf(t, f, a), inv)
	if err != nil || got != acc {
		t.Fatalf("own invoice: %q %v", got, err)
	}
	if _, err := f.reader.AccountOfInvoice(f.ctx, scopeOf(t, f, a), invB); !errors.Is(err, mikrotik.ErrAccountNotFound) {
		t.Fatalf("foreign invoice: %v", err)
	}
}
