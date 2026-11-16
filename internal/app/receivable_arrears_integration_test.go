//go:build integration

package app

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
)

func TestAnyTriggerProduct_OnlyEnabledTriggersOfTheOrg(t *testing.T) {
	f := setupUsageFixture(t)
	a, b := f.tenants.A.OrganizationID, f.tenants.B.OrganizationID
	sa, sb := scopeOf(t, f, a), scopeOf(t, f, b)
	f.trigger(t, a, f.prodP, true, f.today)
	f.trigger(t, a, f.prodQ, false, f.today)

	cases := []struct {
		name string
		ids  []string
		want bool
	}{
		{"enabled product", []string{f.prodP}, true},
		{"disabled product", []string{f.prodQ}, false},
		{"mixed", []string{f.prodQ, f.prodP}, true},
		{"empty", []string{}, false},
	}
	for _, c := range cases {
		got, err := f.reader.AnyTriggerProduct(f.ctx, sa, c.ids)
		if err != nil || got != c.want {
			t.Fatalf("%s: got %v err=%v, want %v", c.name, got, err, c.want)
		}
	}
	if got, err := f.reader.AnyTriggerProduct(f.ctx, sb, []string{f.prodP}); err != nil || got {
		t.Fatalf("org B sees org A trigger: %v %v", got, err)
	}
}

// arrearsContract inserts a contract for the account with one item per product and a recording.
func (f usageFixture) arrearsContract(t *testing.T, org, account, number string, products ...string) (contract, recording string) {
	t.Helper()
	f.tx(org, func(tx pgx.Tx) {
		if err := tx.QueryRow(f.ctx, `
			INSERT INTO receivable_contracts (organization_id, account_id, contract_number, start_date)
			VALUES ($1, $2, $3, $4::date) RETURNING id::text`, org, account, number, f.today.AddDate(-1, 0, 0)).Scan(&contract); err != nil {
			t.Fatalf("contract: %v", err)
		}
		for _, p := range products {
			if _, err := tx.Exec(f.ctx, `
				INSERT INTO receivable_contract_items (organization_id, contract_id, description, product_id, billing_frequency,
					payment_timing, next_period_start, next_period_end)
				VALUES ($1, $2, 'Internet', $3::uuid, 'monthly', 'prepaid', $4::date, $4::date)`, org, contract, p, f.today); err != nil {
				t.Fatalf("contract item: %v", err)
			}
		}
		if err := tx.QueryRow(f.ctx, `
			INSERT INTO receivable_arrears_recordings (organization_id, account_id, contract_id, reason, idempotency_key)
			VALUES ($1, $2, $3, 'lama', gen_random_uuid()::text) RETURNING id::text`, org, account, contract).Scan(&recording); err != nil {
			t.Fatalf("recording: %v", err)
		}
	})
	return contract, recording
}

// arrearsInvoice inserts an arrears invoice (one item without product, as the service creates it).
func (f usageFixture) arrearsInvoice(t *testing.T, org, account, contract, recording, status, number string, due time.Time, flag bool) string {
	t.Helper()
	var id string
	f.tx(org, func(tx pgx.Tx) {
		if err := tx.QueryRow(f.ctx, `
			INSERT INTO receivable_invoices (organization_id, account_id, invoice_number, status, due_date, grand_total, amount_paid,
				source_type, source_id, contract_id, arrears_recording_id, isolir_flag, period_start, period_end)
			VALUES ($1, $2, $3, $4, $5::date, 150000, 0, 'contract', $6, $6, $7, $8, $5::date, $5::date) RETURNING id::text`,
			org, account, number, status, due, contract, recording, flag).Scan(&id); err != nil {
			t.Fatalf("arrears invoice: %v", err)
		}
		if _, err := tx.Exec(f.ctx, `
			INSERT INTO receivable_invoice_items (organization_id, invoice_id, description) VALUES ($1, $2, 'Tunggakan')`, org, id); err != nil {
			t.Fatalf("arrears item: %v", err)
		}
	})
	return id
}

func TestTriggerInvoices_ArrearsFlag(t *testing.T) {
	f := setupUsageFixture(t)
	a := f.tenants.A.OrganizationID
	sa := scopeOf(t, f, a)
	var disabledProd string
	if err := f.reader.db.QueryRow(f.ctx, `SELECT gen_random_uuid()::text`).Scan(&disabledProd); err != nil {
		t.Fatal(err)
	}
	// effective_from is today: an old due date is before it, which the flag deliberately ignores.
	f.trigger(t, a, f.prodP, true, f.today)
	f.trigger(t, a, disabledProd, false, f.today.AddDate(-2, 0, 0))
	old := f.today.AddDate(0, -3, 0)

	accFlagged, accPlain, accDisabled, accPaid := f.account(t, a, "flagged"), f.account(t, a, "plain"), f.account(t, a, "disabled"), f.account(t, a, "paid")
	cF, rF := f.arrearsContract(t, a, accFlagged, "CTR-F", f.prodP)
	cP, rP := f.arrearsContract(t, a, accPlain, "CTR-P", f.prodP)
	cD, rD := f.arrearsContract(t, a, accDisabled, "CTR-D", disabledProd)
	cX, rX := f.arrearsContract(t, a, accPaid, "CTR-X", f.prodP)

	flagged := f.arrearsInvoice(t, a, accFlagged, cF, rF, "overdue", "A-FLAG", old, true)
	f.arrearsInvoice(t, a, accPlain, cP, rP, "overdue", "A-PLAIN", old, false)
	f.arrearsInvoice(t, a, accDisabled, cD, rD, "overdue", "A-DISABLED", old, true)
	f.arrearsInvoice(t, a, accPaid, cX, rX, "paid", "A-PAID", old, true)
	f.arrearsInvoice(t, a, accPaid, cX, rX, "void", "A-VOID", old.AddDate(0, 0, 1), true)

	cases := []struct {
		name, account string
		want          []string
	}{
		{"flagged, enabled trigger, due before effective_from", accFlagged, []string{flagged}},
		{"not flagged", accPlain, nil},
		{"flagged but trigger disabled", accDisabled, nil},
		{"flagged but paid or void", accPaid, nil},
	}
	for _, c := range cases {
		got, err := f.reader.TriggerInvoices(f.ctx, sa, c.account, f.today)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(c.want) || (len(got) == 1 && got[0].ID != c.want[0]) {
			t.Fatalf("%s: got %+v, want %v", c.name, got, c.want)
		}
	}
	accounts, err := f.reader.AccountsWithTriggerInvoices(f.ctx, sa, f.today, "", 10)
	if err != nil || len(accounts) != 1 || accounts[0] != accFlagged {
		t.Fatalf("accounts with trigger invoices = %v err=%v", accounts, err)
	}
	rows, _, err := f.reader.ListCandidates(f.ctx, sa, f.today, f.today, mikrotikservice.CandidateFilter{}, 10, 0)
	if err != nil || len(rows) != 1 || rows[0].AccountID != accFlagged || rows[0].Products != "Internet" || rows[0].InvoiceCount != 1 {
		t.Fatalf("candidates = %+v err=%v", rows, err)
	}
	// Disabling the trigger afterwards removes the candidate.
	f.trigger(t, a, f.prodP, false, f.today)
	if got, _ := f.reader.TriggerInvoices(f.ctx, sa, accFlagged, f.today); len(got) != 0 {
		t.Fatalf("after disabling trigger: %+v", got)
	}
}
