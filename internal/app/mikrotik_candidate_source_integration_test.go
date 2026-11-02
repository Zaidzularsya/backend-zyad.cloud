//go:build integration

package app

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/mikrotik/domain"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
)

// setupCandidateFixture extends the usage fixture with cleanup of the mikrotik
// and catalog rows the candidate read model touches. Mikrotik rows reference
// receivable_accounts without cascade, so they are removed first (t.Cleanup
// runs LIFO: this cleanup runs before the usage fixture's).
func setupCandidateFixture(t *testing.T) usageFixture {
	t.Helper()
	f := setupUsageFixture(t)
	clean := func() {
		for _, org := range []string{f.tenants.A.OrganizationID, f.tenants.B.OrganizationID} {
			tx, err := f.reader.db.Begin(f.ctx)
			if err != nil {
				continue
			}
			_, _ = tx.Exec(f.ctx, "SELECT set_config('app.organization_id', $1, true)", org)
			for _, table := range []string{"mikrotik_pilot_accounts", "mikrotik_customer_policies", "mikrotik_ppp_secrets", "mikrotik_routers", "mikrotik_product_triggers", "mikrotik_org_settings", "catalog_products"} {
				_, _ = tx.Exec(f.ctx, "DELETE FROM "+table+" WHERE organization_id = $1", org)
			}
			_ = tx.Commit(f.ctx)
		}
	}
	clean()
	t.Cleanup(clean)
	return f
}

func domainPolicy(exempt bool, until *time.Time) domain.CustomerPolicy {
	return domain.CustomerPolicy{Exempt: exempt, ExemptedUntil: until}
}

func (f usageFixture) router(t *testing.T, org, name string, ready bool) string {
	t.Helper()
	var id string
	f.tx(org, func(tx pgx.Tx) {
		if err := tx.QueryRow(f.ctx, `
			INSERT INTO mikrotik_routers (organization_id, name, host, port, api_type, use_tls, username, password_encrypted, isolir_ready)
			VALUES ($1, $2, '10.0.0.1', 8728, 'binary', false, 'u', 'x', $3) RETURNING id::text`, org, name, ready).Scan(&id); err != nil {
			t.Fatalf("router: %v", err)
		}
	})
	return id
}

type secretSpec struct {
	present, isolated, needsProfile bool
	router                          string
}

func (f usageFixture) secret(t *testing.T, org, account, name string, s secretSpec) {
	t.Helper()
	state := "normal"
	if s.isolated {
		state = "isolated"
	}
	f.tx(org, func(tx pgx.Tx) {
		if _, err := tx.Exec(f.ctx, `
			INSERT INTO mikrotik_ppp_secrets (organization_id, router_id, name, present, isolation_state, needs_original_profile, account_id)
			VALUES ($1, $2::uuid, $3, $4, $5, $6, $7::uuid)`,
			org, s.router, name, s.present, state, s.needsProfile, account); err != nil {
			t.Fatalf("secret: %v", err)
		}
	})
}

func (f usageFixture) policy(t *testing.T, org, account string, exempt bool, until *time.Time) {
	t.Helper()
	f.tx(org, func(tx pgx.Tx) {
		if _, err := tx.Exec(f.ctx, `
			INSERT INTO mikrotik_customer_policies (organization_id, account_id, exempt, exempted_until)
			VALUES ($1, $2::uuid, $3, $4::date)`, org, account, exempt, until); err != nil {
			t.Fatalf("policy: %v", err)
		}
	})
}

func (f usageFixture) pilot(t *testing.T, org, account string) {
	t.Helper()
	f.tx(org, func(tx pgx.Tx) {
		if _, err := tx.Exec(f.ctx, `INSERT INTO mikrotik_pilot_accounts (organization_id, account_id) VALUES ($1, $2::uuid)`, org, account); err != nil {
			t.Fatalf("pilot: %v", err)
		}
	})
}

func (f usageFixture) catalogProduct(t *testing.T, org, id, name string) {
	t.Helper()
	f.tx(org, func(tx pgx.Tx) {
		if _, err := tx.Exec(f.ctx, `INSERT INTO catalog_products (id, organization_id, name) VALUES ($1::uuid, $2, $3)`, id, org, name); err != nil {
			t.Fatalf("catalog product: %v", err)
		}
	})
}

// statusesOf returns the computed status per account id.
func statusesOf(t *testing.T, rows []mikrotikservice.CandidateRow) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, r := range rows {
		out[r.AccountID] = r.Status
	}
	return out
}

// TestCandidateStatus_SQLMatchesGo runs the Go priority table through the SQL
// read model so the two implementations cannot drift.
func TestCandidateStatus_SQLMatchesGo(t *testing.T) {
	f := setupCandidateFixture(t)
	a := f.tenants.A.OrganizationID
	sa := scopeOf(t, f, a)
	f.trigger(t, a, f.prodP, true, f.today.AddDate(0, 0, -60))
	ready, notReady := f.router(t, a, "ready", true), f.router(t, a, "not-ready", false)
	yesterday, today := f.today.AddDate(0, 0, -1), f.today

	S := func(present, isolated, needs, onReady bool) mikrotikservice.CandidateSecret {
		return mikrotikservice.CandidateSecret{Present: present, Isolated: isolated, NeedsOriginalProfile: needs, RouterReady: onReady}
	}
	cases := []struct {
		name    string
		exempt  bool
		until   *time.Time
		secrets []mikrotikservice.CandidateSecret
	}{
		{"will isolate", false, nil, []mikrotikservice.CandidateSecret{S(true, false, false, true)}},
		{"exempt forever", true, nil, []mikrotikservice.CandidateSecret{S(true, false, false, true)}},
		{"exempt until today", true, &today, []mikrotikservice.CandidateSecret{S(true, false, false, true)}},
		{"exempt until yesterday", true, &yesterday, []mikrotikservice.CandidateSecret{S(true, false, false, true)}},
		{"no secret", false, nil, nil},
		{"exempt + no secret", true, nil, nil},
		{"already isolated", false, nil, []mikrotikservice.CandidateSecret{S(true, true, false, true), S(false, true, false, false)}},
		{"isolated + needs on isolated", false, nil, []mikrotikservice.CandidateSecret{S(true, true, true, true)}},
		{"partly isolated + needs", false, nil, []mikrotikservice.CandidateSecret{S(true, true, false, true), S(true, false, true, true)}},
		{"needs + missing", false, nil, []mikrotikservice.CandidateSecret{S(false, false, true, true)}},
		{"secret missing", false, nil, []mikrotikservice.CandidateSecret{S(false, false, false, true)}},
		{"router not ready", false, nil, []mikrotikservice.CandidateSecret{S(true, false, false, false)}},
		{"present unready, missing ready", false, nil, []mikrotikservice.CandidateSecret{S(true, false, false, false), S(false, false, false, true)}},
		{"one ready among unready", false, nil, []mikrotikservice.CandidateSecret{S(true, false, false, false), S(true, false, false, true)}},
		{"needs only on isolated secret", false, nil, []mikrotikservice.CandidateSecret{S(true, true, true, true), S(false, false, false, true)}},
		{"exempt false with future until", false, &today, []mikrotikservice.CandidateSecret{S(true, false, false, true)}},
	}
	want := map[string]string{}
	var ids []string
	for i, c := range cases {
		acc := f.account(t, a, fmt.Sprintf("tbl-%02d", i))
		ids = append(ids, acc)
		f.invoiceNo(t, a, acc, "issued", fmt.Sprintf("TBL-%02d", i), f.today.AddDate(0, 0, -5), "0", f.prodP)
		if c.exempt || c.until != nil {
			f.policy(t, a, acc, c.exempt, c.until)
		}
		for j, s := range c.secrets {
			rt := notReady
			if s.RouterReady {
				rt = ready
			}
			f.secret(t, a, acc, fmt.Sprintf("tbl-%02d-%d", i, j), secretSpec{present: s.Present, isolated: s.Isolated, needsProfile: s.NeedsOriginalProfile, router: rt})
		}
		exempt := mikrotikservice.EffectiveExempt(domainPolicy(c.exempt, c.until), f.today)
		want[acc] = mikrotikservice.CandidateStatusOf(exempt, c.secrets)
	}
	rows, total, err := f.reader.ListCandidates(f.ctx, sa, f.today, f.today, mikrotikservice.CandidateFilter{AccountIDs: ids}, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != int64(len(cases)) {
		t.Fatalf("total = %d", total)
	}
	got := statusesOf(t, rows)
	for i, c := range cases {
		if got[ids[i]] != want[ids[i]] {
			t.Errorf("%s: sql %q go %q", c.name, got[ids[i]], want[ids[i]])
		}
	}
	// A secret on a deleted router does not count as paired.
	acc := f.account(t, a, "deleted-router")
	f.invoiceNo(t, a, acc, "issued", "TBL-DEL", f.today.AddDate(0, 0, -5), "0", f.prodP)
	gone := f.router(t, a, "gone", true)
	f.secret(t, a, acc, "tbl-del", secretSpec{present: true, router: gone})
	f.tx(a, func(tx pgx.Tx) {
		if _, err := tx.Exec(f.ctx, `UPDATE mikrotik_routers SET deleted_at = now() WHERE id = $1::uuid`, gone); err != nil {
			t.Fatal(err)
		}
	})
	rows, _, err = f.reader.ListCandidates(f.ctx, sa, f.today, f.today, mikrotikservice.CandidateFilter{AccountIDs: []string{acc}}, 10, 0)
	if err != nil || len(rows) != 1 || rows[0].Status != mikrotikservice.CandidateNoSecret {
		t.Fatalf("deleted router: %+v %v", rows, err)
	}
}

func TestCandidates_GraceAndExemptionAndOtherOrg(t *testing.T) {
	f := setupCandidateFixture(t)
	a, b := f.tenants.A.OrganizationID, f.tenants.B.OrganizationID
	sa, sb := scopeOf(t, f, a), scopeOf(t, f, b)
	f.trigger(t, a, f.prodP, true, f.today.AddDate(0, 0, -60))
	f.trigger(t, b, f.prodP, true, f.today.AddDate(0, 0, -60))
	f.catalogProduct(t, a, f.prodP, "Paket 20 Mbps")
	rtA, rtB := f.router(t, a, "R-A", true), f.router(t, b, "R-B", true)

	late := f.account(t, a, "late") // due 5 days ago
	f.invoiceNo(t, a, late, "issued", "G-LATE", f.today.AddDate(0, 0, -5), "50000", f.prodP)
	f.secret(t, a, late, "late", secretSpec{present: true, router: rtA})
	recent := f.account(t, a, "recent") // due 3 days ago
	f.invoiceNo(t, a, recent, "overdue", "G-RECENT", f.today.AddDate(0, 0, -3), "0", f.prodP)
	f.secret(t, a, recent, "recent", secretSpec{present: true, router: rtA})
	expired := f.account(t, a, "expired-exempt")
	f.invoiceNo(t, a, expired, "issued", "G-EXP", f.today.AddDate(0, 0, -5), "0", f.prodP)
	f.secret(t, a, expired, "expired", secretSpec{present: true, router: rtA})
	yesterday := f.today.AddDate(0, 0, -1)
	f.policy(t, a, expired, true, &yesterday)
	exempt := f.account(t, a, "exempt")
	f.invoiceNo(t, a, exempt, "issued", "G-EX", f.today.AddDate(0, 0, -5), "0", f.prodP)
	f.policy(t, a, exempt, true, nil)
	f.pilot(t, a, late)
	// paired, no candidate invoice: a "safe" account
	safe := f.account(t, a, "safe")
	f.secret(t, a, safe, "safe", secretSpec{present: true, router: rtA})
	// org B mirror: must never appear in org A
	accB := f.account(t, b, "late-b")
	f.invoiceNo(t, b, accB, "issued", "G-B", f.today.AddDate(0, 0, -5), "0", f.prodP)
	f.secret(t, b, accB, "late-b", secretSpec{present: true, router: rtB})

	list := func(grace int) map[string]mikrotikservice.CandidateRow {
		t.Helper()
		cutoff := mikrotikservice.CandidateCutoff(f.today, grace)
		rows, total, err := f.reader.ListCandidates(f.ctx, sa, f.today, cutoff, mikrotikservice.CandidateFilter{}, 100, 0)
		if err != nil {
			t.Fatal(err)
		}
		if int(total) != len(rows) {
			t.Fatalf("grace %d: total %d rows %d", grace, total, len(rows))
		}
		out := map[string]mikrotikservice.CandidateRow{}
		for _, r := range rows {
			out[r.AccountID] = r
		}
		return out
	}
	g0 := list(0)
	if len(g0) != 4 || g0[accB].AccountID != "" {
		t.Fatalf("grace 0: %+v", g0)
	}
	if r := g0[late]; r.Status != mikrotikservice.CandidateWillIsolate || r.OverdueDays != 5 || r.InvoiceCount != 1 ||
		r.Balance != "100000.00" || r.OldestInvoiceNumber != "G-LATE" || r.RouterName != "R-A" || !r.Pilot ||
		r.Products != "Paket 20 Mbps" || r.AccountName != "late" {
		t.Fatalf("late row: %+v", r)
	}
	if g0[expired].Status != mikrotikservice.CandidateWillIsolate || g0[expired].ExemptUntil == nil {
		t.Fatalf("expired exemption must not exempt: %+v", g0[expired])
	}
	if g0[exempt].Status != mikrotikservice.CandidateExempt {
		t.Fatalf("exempt: %+v", g0[exempt])
	}
	// grace 3: cutoff = today-3, the invoice due today-3 is no longer a candidate.
	g3 := list(3)
	if _, ok := g3[recent]; ok || len(g3) != 3 {
		t.Fatalf("grace 3: %+v", g3)
	}
	// grace 5: nothing qualifies.
	if g5 := list(5); len(g5) != 0 {
		t.Fatalf("grace 5: %+v", g5)
	}

	sum, err := f.reader.Summary(f.ctx, sa, f.today, f.today)
	if err != nil {
		t.Fatal(err)
	}
	// target: late, recent, expired; exempt: exempt (no secret, still exempt);
	// paired: late, recent, expired, safe.
	if sum.Target != 3 || sum.Exempt != 1 || sum.Blocked != 0 || sum.AlreadyIsolated != 0 || sum.PairedAccounts != 4 {
		t.Fatalf("summary: %+v", sum)
	}
	// Summary agrees with the list per status.
	for _, st := range []string{mikrotikservice.CandidateWillIsolate, mikrotikservice.CandidateExempt} {
		_, n, err := f.reader.ListCandidates(f.ctx, sa, f.today, f.today, mikrotikservice.CandidateFilter{Status: st}, 1, 0)
		if err != nil {
			t.Fatal(err)
		}
		if (st == mikrotikservice.CandidateWillIsolate && n != 3) || (st == mikrotikservice.CandidateExempt && n != 1) {
			t.Fatalf("status %s total %d", st, n)
		}
	}
	// Search: case-insensitive, LIKE wildcards are literal.
	rows, n, err := f.reader.ListCandidates(f.ctx, sa, f.today, f.today, mikrotikservice.CandidateFilter{Q: "LAT"}, 10, 0)
	if err != nil || n != 1 || rows[0].AccountID != late {
		t.Fatalf("q: %+v %d %v", rows, n, err)
	}
	if _, n, err := f.reader.ListCandidates(f.ctx, sa, f.today, f.today, mikrotikservice.CandidateFilter{Q: "%"}, 10, 0); err != nil || n != 0 {
		t.Fatalf("q %%: %d %v", n, err)
	}

	// org B sees only its own candidate.
	rowsB, nB, err := f.reader.ListCandidates(f.ctx, sb, f.today, f.today, mikrotikservice.CandidateFilter{}, 100, 0)
	if err != nil || nB != 1 || rowsB[0].AccountID != accB {
		t.Fatalf("org B: %+v %d %v", rowsB, nB, err)
	}
	// org A ids requested under org B scope: nothing.
	if rows, _, err := f.reader.ListCandidates(f.ctx, sb, f.today, f.today, mikrotikservice.CandidateFilter{AccountIDs: []string{late}}, 10, 0); err != nil || len(rows) != 0 {
		t.Fatalf("cross-org ids: %+v %v", rows, err)
	}
	sumB, err := f.reader.Summary(f.ctx, sb, f.today, f.today)
	if err != nil || sumB.Target != 1 || sumB.PairedAccounts != 1 {
		t.Fatalf("summary B: %+v %v", sumB, err)
	}
}

func TestCandidates_Pagination_StableOrder(t *testing.T) {
	f := setupCandidateFixture(t)
	a := f.tenants.A.OrganizationID
	sa := scopeOf(t, f, a)
	f.trigger(t, a, f.prodP, true, f.today.AddDate(0, 0, -60))
	type key struct {
		due time.Time
		id  string
	}
	var want []key
	for i := 0; i < 7; i++ {
		acc := f.account(t, a, fmt.Sprintf("pg-%d", i))
		due := f.today.AddDate(0, 0, -1-(i%3)) // ties on due date
		f.invoiceNo(t, a, acc, "issued", fmt.Sprintf("PG-%d", i), due, "0", f.prodP)
		if i == 0 { // a second, older invoice decides the oldest due date
			due = f.today.AddDate(0, 0, -10)
			f.invoiceNo(t, a, acc, "issued", "PG-0B", due, "0", f.prodP)
		}
		want = append(want, key{due, acc})
	}
	sort.Slice(want, func(i, j int) bool {
		if !want[i].due.Equal(want[j].due) {
			return want[i].due.Before(want[j].due)
		}
		return want[i].id < want[j].id
	})
	var got []string
	for off := 0; off < 10; off += 3 {
		rows, total, err := f.reader.ListCandidates(f.ctx, sa, f.today, f.today, mikrotikservice.CandidateFilter{}, 3, off)
		if err != nil {
			t.Fatal(err)
		}
		if total != 7 {
			t.Fatalf("offset %d total %d", off, total)
		}
		for _, r := range rows {
			got = append(got, r.AccountID)
			if r.AccountName == "pg-0" && (r.InvoiceCount != 2 || r.OverdueDays != 10 || r.OldestInvoiceNumber != "PG-0B" || r.Balance != "300000.00") {
				t.Fatalf("pg-0 row: %+v", r)
			}
		}
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows", len(got))
	}
	for i := range want {
		if got[i] != want[i].id {
			t.Fatalf("position %d: got %s want %s", i, got[i], want[i].id)
		}
	}
}
