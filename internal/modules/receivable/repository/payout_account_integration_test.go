//go:build integration

package repository_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"zyad.cloud/internal/modules/receivable/repository"
)

type payoutEnv struct {
	env
	repo    repository.PayoutAccountRepository
	ledger  repository.SettlementLedgerRepository
	actorID string
	audit   repository.PayoutAudit
}

// setupPayout membersihkan rekening, entri ledger, dan audit org A/B sebelum dan sesudah tes.
// settlement_entries dihapus lebih dulu karena FK payout_account_id.
func setupPayout(t *testing.T) payoutEnv {
	t.Helper()
	e := setup(t)
	orgA, orgB := e.tenants.A.OrganizationID, e.tenants.B.OrganizationID
	var actorID string
	if err := e.db.QueryRow(e.ctx, `INSERT INTO users (name, email, status) VALUES ('Payout Admin', $1, 'active') RETURNING id::text`,
		"payout-it-"+time.Now().UTC().Format("20060102150405.000000000")+"@example.test").Scan(&actorID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	clean := func() {
		_, _ = e.db.Exec(e.ctx, `DELETE FROM settlement_entries WHERE beneficiary_organization_id IN ($1, $2)`, orgA, orgB)
		_, _ = e.db.Exec(e.ctx, `DELETE FROM audit_logs WHERE target_type = 'organization_payout_account' AND organization_id IN ($1, $2)`, orgA, orgB)
		_, _ = e.db.Exec(e.ctx, `DELETE FROM organization_payout_accounts WHERE organization_id IN ($1, $2)`, orgA, orgB)
	}
	clean()
	t.Cleanup(func() {
		clean()
		_, _ = e.db.Exec(e.ctx, `DELETE FROM users WHERE id = $1`, actorID)
	})
	return payoutEnv{
		env: e, repo: repository.NewPayoutAccountRepository(e.db), ledger: repository.NewSettlementLedgerRepository(e.db),
		actorID: actorID, audit: repository.PayoutAudit{ActorUserID: actorID, IPAddress: "203.0.113.7", UserAgent: "it-test"},
	}
}

func (p payoutEnv) newAccount(orgID, number string) repository.NewPayoutAccount {
	return repository.NewPayoutAccount{
		OrganizationID: orgID, BankCode: "BCA", BankName: "Bank Central Asia", AccountNumber: number,
		AccountHolderName: "PT Contoh", ActorUserID: p.actorID, At: time.Now().UTC(),
	}
}

func (p payoutEnv) submit(t *testing.T, orgID, number string) repository.PayoutAccount {
	t.Helper()
	acc, err := p.repo.InsertPending(p.ctx, p.newAccount(orgID, number), p.audit)
	if err != nil {
		t.Fatalf("InsertPending: %v", err)
	}
	return acc
}

func (p payoutEnv) verified(t *testing.T, orgID, number string) repository.PayoutAccount {
	t.Helper()
	acc, err := p.repo.Verify(p.ctx, p.submit(t, orgID, number).ID, "ok", time.Now().UTC(), p.audit)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	return acc
}

func (p payoutEnv) status(t *testing.T, id string) string {
	t.Helper()
	acc, err := p.repo.Find(p.ctx, id)
	if err != nil {
		t.Fatalf("Find %s: %v", id, err)
	}
	return acc.Status
}

func (p payoutEnv) seedPayment(t *testing.T, orgID, amount string) {
	t.Helper()
	inv := p.issued(t, p.tenants.A.Scope, p.account(t, p.tenants.A.Scope, "Budi").ID, "INV-2026-0971")
	if _, err := repository.NewSettlementRepository(p.db).InsertPayment(p.ctx, repository.SettlementPaymentParams{
		BeneficiaryOrganizationID: orgID, InvoiceID: inv.ID, ProviderReference: "it-payout-" + inv.ID, Amount: amount, OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPayoutVerifySupersedesPrevious(t *testing.T) {
	p := setupPayout(t)
	orgA := p.tenants.A.OrganizationID
	a := p.verified(t, orgA, "1111222233")
	if a.Status != "verified" || a.ReviewedBy != p.actorID || a.ReviewedAt == nil || a.ReviewNote != "ok" {
		t.Fatalf("verified A = %+v", a)
	}

	pendingB := p.submit(t, orgA, "4444555566")
	if _, err := p.repo.InsertPending(p.ctx, p.newAccount(orgA, "7777888899"), p.audit); !errors.Is(err, repository.ErrPayoutAccountPendingExists) {
		t.Fatalf("second pending err = %v", err)
	}
	b, err := p.repo.Verify(p.ctx, pendingB.ID, "", time.Now().UTC(), p.audit)
	if err != nil || b.Status != "verified" {
		t.Fatalf("verify B = %+v err=%v", b, err)
	}
	if got := p.status(t, a.ID); got != "superseded" {
		t.Fatalf("A status = %s, want superseded", got)
	}
	list, err := p.repo.ListByOrganization(p.ctx, orgA)
	if err != nil || len(list) != 2 || list[0].ID != b.ID {
		t.Fatalf("list = %+v err=%v (terbaru dulu)", list, err)
	}
	sum, err := p.repo.SummaryForOrganizations(p.ctx, []string{orgA, p.tenants.B.OrganizationID})
	if err != nil || sum[orgA].Verified == nil || sum[orgA].Verified.ID != b.ID || sum[orgA].HasPending {
		t.Fatalf("summary = %+v err=%v", sum, err)
	}
	if s := sum[p.tenants.B.OrganizationID]; s.Verified != nil || s.HasPending {
		t.Fatalf("summary B = %+v", s)
	}
}

func TestPayoutAssignCancelsPendingAndSupersedes(t *testing.T) {
	p := setupPayout(t)
	orgA := p.tenants.A.OrganizationID
	old := p.verified(t, orgA, "1111222233")
	pending := p.submit(t, orgA, "4444555566")

	pend, total, err := p.repo.ListPending(p.ctx, 200, 0)
	if err != nil || total < 1 {
		t.Fatalf("ListPending total=%d err=%v", total, err)
	}
	found := false
	for _, row := range pend {
		if row.ID == pending.ID {
			found = row.OrganizationName == "Organization A"
		}
	}
	if !found {
		t.Fatalf("pending A tidak ada di antrean (atau nama org salah): %+v", pend)
	}

	assigned, err := p.repo.AssignVerified(p.ctx, p.newAccount(orgA, "9999000011"), "ditetapkan admin", p.audit)
	if err != nil {
		t.Fatal(err)
	}
	if assigned.Status != "verified" || assigned.SubmittedBy != p.actorID || assigned.ReviewedBy != p.actorID || assigned.ReviewNote != "ditetapkan admin" {
		t.Fatalf("assigned = %+v", assigned)
	}
	if got := p.status(t, old.ID); got != "superseded" {
		t.Fatalf("old = %s", got)
	}
	if got := p.status(t, pending.ID); got != "cancelled" {
		t.Fatalf("pending = %s", got)
	}
	// Tanpa rekening sebelumnya juga boleh.
	if _, err := p.repo.AssignVerified(p.ctx, p.newAccount(p.tenants.B.OrganizationID, "5555666677"), "awal", p.audit); err != nil {
		t.Fatal(err)
	}
}

func TestPayoutCancelRejectsOtherOrg(t *testing.T) {
	p := setupPayout(t)
	acc := p.submit(t, p.tenants.A.OrganizationID, "1111222233")
	if _, err := p.repo.Cancel(p.ctx, p.tenants.B.OrganizationID, acc.ID, time.Now().UTC(), p.audit); !errors.Is(err, repository.ErrPayoutAccountNotFound) {
		t.Fatalf("cross-org cancel err = %v", err)
	}
	if got := p.status(t, acc.ID); got != "pending" {
		t.Fatalf("status = %s", got)
	}
	if _, err := p.repo.Find(p.ctx, "not-a-uuid"); !errors.Is(err, repository.ErrPayoutAccountNotFound) {
		t.Fatalf("invalid id err = %v", err)
	}
	cancelled, err := p.repo.Cancel(p.ctx, p.tenants.A.OrganizationID, acc.ID, time.Now().UTC(), p.audit)
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("cancel = %+v err=%v", cancelled, err)
	}
	if _, err := p.repo.Cancel(p.ctx, p.tenants.A.OrganizationID, acc.ID, time.Now().UTC(), p.audit); !errors.Is(err, repository.ErrPayoutAccountInvalidState) {
		t.Fatalf("cancel twice err = %v", err)
	}
}

func TestPayoutRejectRequiresPending(t *testing.T) {
	p := setupPayout(t)
	acc := p.submit(t, p.tenants.A.OrganizationID, "1111222233")
	rejected, err := p.repo.Reject(p.ctx, acc.ID, "nama tidak cocok", time.Now().UTC(), p.audit)
	if err != nil || rejected.Status != "rejected" || rejected.ReviewNote != "nama tidak cocok" || rejected.ReviewedBy != p.actorID {
		t.Fatalf("reject = %+v err=%v", rejected, err)
	}
	if _, err := p.repo.Reject(p.ctx, acc.ID, "lagi", time.Now().UTC(), p.audit); !errors.Is(err, repository.ErrPayoutAccountInvalidState) {
		t.Fatalf("reject twice err = %v", err)
	}
	if _, err := p.repo.Verify(p.ctx, acc.ID, "", time.Now().UTC(), p.audit); !errors.Is(err, repository.ErrPayoutAccountInvalidState) {
		t.Fatalf("verify rejected err = %v", err)
	}
	if _, err := p.repo.Reject(p.ctx, "00000000-0000-0000-0000-000000000000", "x", time.Now().UTC(), p.audit); !errors.Is(err, repository.ErrPayoutAccountNotFound) {
		t.Fatalf("missing err = %v", err)
	}
}

func TestInsertManualLockedPassesVerifiedPayoutID(t *testing.T) {
	p := setupPayout(t)
	orgA := p.tenants.A.OrganizationID
	p.seedPayment(t, orgA, "1000.00")
	in := func(kind, amount string) repository.SettlementManualEntry {
		return repository.SettlementManualEntry{OrganizationID: orgA, Kind: kind, Amount: amount, Note: "n", OccurredAt: time.Now().UTC()}
	}

	var seen []string
	record := func(t repository.SettlementTotals, id string) error { seen = append(seen, id); return nil }
	e, err := p.ledger.InsertManualLocked(p.ctx, in("settlement", "1.00"), record)
	if err != nil || e.PayoutAccountID != "" {
		t.Fatalf("no verified: entry=%+v err=%v", e, err)
	}
	// Rekening pending (atau milik org lain) tidak dianggap.
	p.submit(t, orgA, "1111222233")
	p.verified(t, p.tenants.B.OrganizationID, "5555666677")
	if _, err := p.ledger.InsertManualLocked(p.ctx, in("settlement", "1.00"), record); err != nil {
		t.Fatal(err)
	}

	acc, err := p.repo.AssignVerified(p.ctx, p.newAccount(orgA, "4444555566"), "admin", p.audit)
	if err != nil {
		t.Fatal(err)
	}
	e, err = p.ledger.InsertManualLocked(p.ctx, in("settlement", "2.00"), record)
	if err != nil || e.PayoutAccountID != acc.ID || e.PayoutBankName != "Bank Central Asia" || e.PayoutAccountNumber != "4444555566" {
		t.Fatalf("with verified: entry=%+v err=%v", e, err)
	}
	adj, err := p.ledger.InsertManualLocked(p.ctx, in("adjustment", "-1.00"), record)
	if err != nil || adj.PayoutAccountID != "" {
		t.Fatalf("adjustment entry=%+v err=%v", adj, err)
	}
	if want := []string{"", "", acc.ID, acc.ID}; len(seen) != 4 || seen[0] != want[0] || seen[1] != want[1] || seen[2] != want[2] || seen[3] != want[3] {
		t.Fatalf("callback ids = %v, want %v", seen, want)
	}

	var stored string
	if err := p.db.QueryRow(p.ctx, `SELECT COALESCE(payout_account_id::text, '') FROM settlement_entries WHERE id = $1`, e.ID).Scan(&stored); err != nil || stored != acc.ID {
		t.Fatalf("stored payout_account_id = %q err=%v", stored, err)
	}
	entries, _, err := p.ledger.ListEntries(p.ctx, orgA, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	var listed repository.SettlementEntry
	for _, row := range entries {
		if row.ID == e.ID {
			listed = row
		}
	}
	if listed.PayoutAccountID != acc.ID || listed.PayoutBankName != "Bank Central Asia" || listed.PayoutAccountNumber != "4444555566" {
		t.Fatalf("ListEntries row = %+v", listed)
	}
}

// Verify(B) dan setoran berjalan paralel pada org yang sama. Urutan mana pun boleh (A bila setoran
// menang kunci, B bila verify menang), tapi entri tidak boleh memakai A bila Verify(B) sudah commit
// sebelum entri membaca rekening. Urutan diukur dengan jam monotonic Go: verifyDone diambil setelah
// Verify kembali (sudah commit), checkAt diambil di dalam callback (di dalam kunci).
func TestVerifyAndSettlementSerialize(t *testing.T) {
	p := setupPayout(t)
	orgA := p.tenants.A.OrganizationID
	p.seedPayment(t, orgA, "1000.00")
	orders := map[bool]int{}
	for i := 0; i < 5; i++ {
		_, _ = p.db.Exec(p.ctx, `DELETE FROM settlement_entries WHERE beneficiary_organization_id = $1 AND kind = 'settlement'`, orgA)
		_, _ = p.db.Exec(p.ctx, `DELETE FROM organization_payout_accounts WHERE organization_id = $1`, orgA)
		a := p.verified(t, orgA, "1111222233")
		b := p.submit(t, orgA, "4444555566")

		var wg sync.WaitGroup
		var entry repository.SettlementEntry
		var callbackID string
		var verifyDone, checkAt time.Time
		var entryErr, verifyErr error
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			_, verifyErr = p.repo.Verify(p.ctx, b.ID, "", time.Now().UTC(), p.audit)
			verifyDone = time.Now()
		}()
		go func() {
			defer wg.Done()
			<-start
			entry, entryErr = p.ledger.InsertManualLocked(p.ctx, repository.SettlementManualEntry{
				OrganizationID: orgA, Kind: "settlement", Amount: "1.00", OccurredAt: time.Now().UTC(),
			}, func(_ repository.SettlementTotals, id string) error {
				callbackID, checkAt = id, time.Now()
				// Menahan kunci sebentar memperbesar peluang Verify menunggu di belakangnya.
				time.Sleep(20 * time.Millisecond)
				return nil
			})
		}()
		close(start)
		wg.Wait()
		if verifyErr != nil || entryErr != nil {
			t.Fatalf("iter %d: verify=%v entry=%v", i, verifyErr, entryErr)
		}
		if entry.PayoutAccountID != callbackID || (callbackID != a.ID && callbackID != b.ID) {
			t.Fatalf("iter %d: entry payout=%s callback=%s a=%s b=%s", i, entry.PayoutAccountID, callbackID, a.ID, b.ID)
		}
		if callbackID == a.ID && verifyDone.Before(checkAt) {
			t.Fatalf("iter %d: entri memakai A padahal Verify(B) sudah commit sebelum entri membaca rekening", i)
		}
		orders[callbackID == b.ID]++
		if p.status(t, a.ID) != "superseded" || p.status(t, b.ID) != "verified" {
			t.Fatalf("iter %d: final statuses a=%s b=%s", i, p.status(t, a.ID), p.status(t, b.ID))
		}
	}
	t.Logf("entri memakai A (setoran duluan): %d, memakai B (verify duluan): %d", orders[false], orders[true])
}

func TestPayoutMutationsWriteAudit(t *testing.T) {
	p := setupPayout(t)
	orgA := p.tenants.A.OrganizationID
	first := p.submit(t, orgA, "1111222233")
	if _, err := p.repo.Cancel(p.ctx, orgA, first.ID, time.Now().UTC(), p.audit); err != nil {
		t.Fatal(err)
	}
	second := p.submit(t, orgA, "4444555566")
	if _, err := p.repo.Reject(p.ctx, second.ID, "salah", time.Now().UTC(), p.audit); err != nil {
		t.Fatal(err)
	}
	third := p.submit(t, orgA, "7777888899")
	if _, err := p.repo.Verify(p.ctx, third.ID, "", time.Now().UTC(), p.audit); err != nil {
		t.Fatal(err)
	}
	assigned, err := p.repo.AssignVerified(p.ctx, p.newAccount(orgA, "9999000011"), "admin", p.audit)
	if err != nil {
		t.Fatal(err)
	}

	rows, err := p.db.Query(p.ctx, `SELECT module, event, target_id::text, COALESCE(actor_user_id::text, ''),
		COALESCE(host(ip_address), ''), COALESCE(user_agent, ''), metadata
		FROM audit_logs WHERE target_type = 'organization_payout_account' AND organization_id = $1
		ORDER BY created_at, event`, orgA)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	events := map[string]string{}
	for rows.Next() {
		var module, event, targetID, actor, ip, ua string
		var raw []byte
		if err := rows.Scan(&module, &event, &targetID, &actor, &ip, &ua, &raw); err != nil {
			t.Fatal(err)
		}
		var meta map[string]any
		if err := json.Unmarshal(raw, &meta); err != nil {
			t.Fatal(err)
		}
		if module != "receivable" || actor != p.actorID || ip != "203.0.113.7" || ua != "it-test" {
			t.Fatalf("audit row %s: module=%s actor=%s ip=%s ua=%s", event, module, actor, ip, ua)
		}
		if len(meta) != 2 || meta["bank_code"] != "BCA" {
			t.Fatalf("audit %s metadata = %v (hanya bank_code + account_number_masked)", event, meta)
		}
		masked, _ := meta["account_number_masked"].(string)
		if len([]rune(masked)) != 8 || masked[:len("••••")] != "••••" {
			t.Fatalf("audit %s masked = %q", event, masked)
		}
		events[event+":"+targetID] = masked
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"payout_account.submitted:" + first.ID:            "••••2233",
		"payout_account.cancelled:" + first.ID:            "••••2233",
		"payout_account.submitted:" + second.ID:           "••••5566",
		"payout_account.rejected:" + second.ID:            "••••5566",
		"payout_account.submitted:" + third.ID:            "••••8899",
		"payout_account.verified:" + third.ID:             "••••8899",
		"payout_account.assigned_by_admin:" + assigned.ID: "••••0011",
	}
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for k, v := range want {
		if events[k] != v {
			t.Fatalf("event %s = %q, want %q (all=%v)", k, events[k], v, events)
		}
	}
}
