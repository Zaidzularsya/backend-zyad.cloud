//go:build integration

package repository_test

import (
	"sort"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
)

// ListIsolated pages only this organization's isolated secrets on routers
// that are not deleted, ordered by id.
func TestSecretRepo_ListIsolated_KeysetScopedAndSkipsDeletedRouters(t *testing.T) {
	e := setupRouter(t)
	scope, other := e.tenants.A.Scope, e.tenants.B.Scope
	repo := secretRepo(e)

	r1 := e.create(t, scope, "R-iso-1")
	var want []string
	for _, name := range []string{"a", "b", "c"} {
		id := pairedSecret(t, e, scope, r1.ID, name)
		if ok, err := repo.MarkIsolated(e.ctx, scope, id, ""); err != nil || !ok {
			t.Fatalf("isolate %s: ok=%v err=%v", name, ok, err)
		}
		want = append(want, id)
	}
	sort.Strings(want)
	pairedSecret(t, e, scope, r1.ID, "normal")

	r2 := e.create(t, scope, "R-iso-deleted")
	gone := pairedSecret(t, e, scope, r2.ID, "gone")
	if ok, err := repo.MarkIsolated(e.ctx, scope, gone, ""); err != nil || !ok {
		t.Fatalf("isolate gone: ok=%v err=%v", ok, err)
	}
	// SoftDelete refuses a router with isolated secrets; mark it deleted directly.
	tx, err := e.db.Begin(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(e.ctx)
	if _, err := tx.Exec(e.ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID()); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(e.ctx, `UPDATE mikrotik_routers SET deleted_at = now() WHERE organization_id = $1 AND id = $2`, scope.OrganizationID(), r2.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(e.ctx); err != nil {
		t.Fatal(err)
	}

	rb := e.create(t, other, "R-iso-other")
	foreign := pairedSecret(t, e, other, rb.ID, "foreign")
	if ok, err := repo.MarkIsolated(e.ctx, other, foreign, ""); err != nil || !ok {
		t.Fatalf("isolate foreign: ok=%v err=%v", ok, err)
	}

	first, err := repo.ListIsolated(e.ctx, scope, "", 2)
	if err != nil || len(first) != 2 || first[0].ID != want[0] || first[1].ID != want[1] {
		t.Fatalf("first page: err=%v items=%+v want %v", err, first, want)
	}
	second, err := repo.ListIsolated(e.ctx, scope, first[1].ID, 2)
	if err != nil || len(second) != 1 || second[0].ID != want[2] {
		t.Fatalf("second page: err=%v items=%+v", err, second)
	}
	last, err := repo.ListIsolated(e.ctx, scope, second[0].ID, 2)
	if err != nil || len(last) != 0 {
		t.Fatalf("last page: err=%v items=%+v", err, last)
	}
	b, err := repo.ListIsolated(e.ctx, other, "", 10)
	if err != nil || len(b) != 1 || b[0].ID != foreign {
		t.Fatalf("organization B: err=%v items=%+v", err, b)
	}

	// CountIsolated uses exactly the ListIsolated filters.
	for _, tc := range []struct {
		name string
		n    int
		sc   coretenant.Scope
	}{{"A", len(want), scope}, {"B", len(b), other}} {
		got, err := repo.CountIsolated(e.ctx, tc.sc)
		if err != nil || got != tc.n {
			t.Fatalf("CountIsolated %s = %d err=%v, want %d", tc.name, got, err, tc.n)
		}
	}
}
