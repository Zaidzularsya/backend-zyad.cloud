//go:build integration

package repository_test

import (
	"testing"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

// seedSummaryData fills one router with: 1 paired, 1 unpaired, 1 missing,
// 1 isolated+paired, 1 needs_original_profile+paired, plus one pending task
// (on the paired secret) and one failed task (on the unpaired secret).
func seedSummaryData(t *testing.T, e routerEnv, scope coretenant.Scope, routerID string) {
	t.Helper()
	paired := pairedSecret(t, e, scope, routerID, "paired")
	unpaired := seedSecret(t, e, scope, routerID, repository.ImportedSecret{Name: "unpaired", Profile: "p"})
	missing := seedSecret(t, e, scope, routerID, repository.ImportedSecret{Name: "missing", Profile: "p"})
	if err := secretRepo(e).MarkAbsent(e.ctx, scope, missing); err != nil {
		t.Fatal(err)
	}
	isolated := pairedSecret(t, e, scope, routerID, "isolated")
	if ok, err := secretRepo(e).MarkIsolated(e.ctx, scope, isolated, ""); err != nil || !ok {
		t.Fatalf("mark isolated: %v %v", ok, err)
	}
	needs := pairedSecret(t, e, scope, routerID, "needs")
	secretScopedTx(t, e, scope, func(tx pgx.Tx) {
		if _, err := tx.Exec(e.ctx, `UPDATE mikrotik_ppp_secrets SET needs_original_profile = true WHERE id = $1`, needs); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(e.ctx, `INSERT INTO mikrotik_isolation_tasks (organization_id, secret_id, action, status)
			VALUES ($1, $2, 'isolate', 'failed')`, scope.OrganizationID(), unpaired); err != nil {
			t.Fatal(err)
		}
		_ = tx.Commit(e.ctx)
	})
	if ok, err := taskRepo(e).EnqueueIsolate(e.ctx, scope, paired, "test"); err != nil || !ok {
		t.Fatalf("enqueue: %v %v", ok, err)
	}
}

func TestRouterSummaryCountsPerOrganization(t *testing.T) {
	e := setupRouter(t)
	a, b := e.tenants.A.Scope, e.tenants.B.Scope
	ra := e.create(t, a, "R-A")
	if err := e.repo.MarkSetup(e.ctx, a, ra.ID, true, "isolir", ""); err != nil {
		t.Fatal(err)
	}
	rb := e.create(t, b, "R-B")
	if err := e.repo.MarkSetup(e.ctx, b, rb.ID, true, "isolir", ""); err != nil {
		t.Fatal(err)
	}
	seedSummaryData(t, e, a, ra.ID)
	seedSummaryData(t, e, b, rb.ID)

	got, err := e.repo.Summary(e.ctx, a, ra.ID)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	want := domain.RouterSummary{
		SecretsTotal: 5, SecretsPaired: 3, SecretsUnpaired: 1, SecretsMissing: 1,
		SecretsNeedOriginalProfile: 1, SecretsIsolated: 1,
	}
	if got.SecretsTotal != want.SecretsTotal || got.SecretsPaired != want.SecretsPaired ||
		got.SecretsUnpaired != want.SecretsUnpaired || got.SecretsMissing != want.SecretsMissing ||
		got.SecretsNeedOriginalProfile != want.SecretsNeedOriginalProfile || got.SecretsIsolated != want.SecretsIsolated {
		t.Fatalf("secret counts = %+v, want %+v", got, want)
	}
	if got.TaskCounts["pending"] != 1 || got.TaskCounts["failed"] != 1 || len(got.TaskCounts) != 2 {
		t.Fatalf("task counts = %v", got.TaskCounts)
	}

	// Another organization's router is invisible through this scope.
	if _, err := e.repo.Summary(e.ctx, a, rb.ID); err == nil {
		t.Fatal("expected not found for router of another organization")
	}
}
