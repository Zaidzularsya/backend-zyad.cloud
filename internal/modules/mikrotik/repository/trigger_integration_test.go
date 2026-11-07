//go:build integration

package repository_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func d(m time.Month, day int) time.Time { return time.Date(2026, m, day, 0, 0, 0, 0, time.UTC) }

func TestTriggerRepo_UpsertSemanticsAndRLS(t *testing.T) {
	e := setupRouter(t)
	a, b := e.tenants.A.Scope, e.tenants.B.Scope
	repo := policyRepo(e)
	pid := "00000000-0000-0000-0000-000000000001"

	if _, found, err := repo.GetProductTrigger(e.ctx, a, pid); err != nil || found {
		t.Fatalf("before: found=%v err=%v", found, err)
	}
	tr, err := repo.UpsertProductTrigger(e.ctx, a, pid, true, d(time.November, 6), "")
	if err != nil || !tr.Enabled || !tr.EffectiveFrom.Equal(d(time.November, 6)) || tr.SetBy != nil {
		t.Fatalf("enable = %+v, %v", tr, err)
	}
	// Idempotent enable keeps the date.
	tr, err = repo.UpsertProductTrigger(e.ctx, a, pid, true, d(time.November, 16), "")
	if err != nil || !tr.EffectiveFrom.Equal(d(time.November, 6)) {
		t.Fatalf("re-enable same = %+v, %v", tr, err)
	}
	// Disable keeps the row and the date.
	tr, err = repo.UpsertProductTrigger(e.ctx, a, pid, false, d(time.November, 20), "")
	if err != nil || tr.Enabled || !tr.EffectiveFrom.Equal(d(time.November, 6)) {
		t.Fatalf("disable = %+v, %v", tr, err)
	}
	// Enable after disable resets the date.
	tr, err = repo.UpsertProductTrigger(e.ctx, a, pid, true, d(time.November, 21), "")
	if err != nil || !tr.Enabled || !tr.EffectiveFrom.Equal(d(time.November, 21)) {
		t.Fatalf("re-enable after disable = %+v, %v", tr, err)
	}
	// A brand-new row disabled stores today's date.
	pid2 := "00000000-0000-0000-0000-000000000002"
	tr, err = repo.UpsertProductTrigger(e.ctx, a, pid2, false, d(time.November, 22), "")
	if err != nil || tr.Enabled || !tr.EffectiveFrom.Equal(d(time.November, 22)) {
		t.Fatalf("new disabled = %+v, %v", tr, err)
	}

	list, err := repo.ListProductTriggers(e.ctx, a)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if got, err := repo.ListProductTriggers(e.ctx, b); err != nil || len(got) != 0 {
		t.Fatalf("cross-org list = %+v, %v", got, err)
	}
	if _, found, err := repo.GetProductTrigger(e.ctx, b, pid); err != nil || found {
		t.Fatalf("cross-org get found=%v err=%v", found, err)
	}
	var n int
	secretScopedTx(t, e, a, func(tx pgx.Tx) {
		_ = tx.QueryRow(e.ctx, `SELECT count(*) FROM mikrotik_product_triggers`).Scan(&n)
	})
	if n != 2 {
		t.Fatalf("rows = %d", n)
	}
}
