//go:build integration

package repository_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

func taskRepo(e routerEnv) repository.TaskRepository { return repository.NewTaskRepository(e.db) }

// isoFixture: a ready router with one paired, present, normal secret.
type isoFixture struct {
	e        routerEnv
	scope    coretenant.Scope
	routerID string
	secretID string
	account  string
}

func newIsoFixture(t *testing.T) isoFixture {
	t.Helper()
	e := setupRouter(t)
	scope := e.tenants.A.Scope
	r := e.create(t, scope, "R1")
	if err := e.repo.MarkSetup(e.ctx, scope, r.ID, true, "isolir", ""); err != nil {
		t.Fatal(err)
	}
	return isoFixture{e: e, scope: scope, routerID: r.ID, secretID: pairedSecret(t, e, scope, r.ID, "alice"), account: ""}
}

func pairedSecret(t *testing.T, e routerEnv, scope coretenant.Scope, routerID, name string) string {
	t.Helper()
	id := seedSecret(t, e, scope, routerID, repository.ImportedSecret{Name: name, Profile: "paket"})
	if err := secretRepo(e).Pair(e.ctx, scope, id, newAccount(t, e, scope), ""); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f isoFixture) pending(t *testing.T) []domain.TaskView {
	t.Helper()
	items, _, err := taskRepo(f.e).List(f.e.ctx, f.scope, repository.TaskFilter{}, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestEnqueueIsolate_SkipsUnpairedMissingNeedsProfileNotReadyRouter(t *testing.T) {
	f := newIsoFixture(t)
	e, scope := f.e, f.scope
	repo := taskRepo(e)

	unpaired := seedSecret(t, e, scope, f.routerID, repository.ImportedSecret{Name: "unpaired", Profile: "p"})
	missing := pairedSecret(t, e, scope, f.routerID, "missing")
	if err := secretRepo(e).MarkAbsent(e.ctx, scope, missing); err != nil {
		t.Fatal(err)
	}
	needs := seedSecret(t, e, scope, f.routerID, repository.ImportedSecret{Name: "needs", Profile: "p"})
	if err := secretRepo(e).Pair(e.ctx, scope, needs, newAccount(t, e, scope), ""); err != nil {
		t.Fatal(err)
	}
	// raise the flag after pairing (the importer does this on a later sync)
	secretScopedTx(t, e, scope, func(tx pgx.Tx) {
		if _, err := tx.Exec(e.ctx, `UPDATE mikrotik_ppp_secrets SET needs_original_profile = true WHERE id = $1`, needs); err != nil {
			t.Fatal(err)
		}
		_ = tx.Commit(e.ctx)
	})
	r2 := e.create(t, scope, "NotReady")
	notReady := pairedSecret(t, e, scope, r2.ID, "bob")
	isolated := pairedSecret(t, e, scope, f.routerID, "iso")
	if ok, err := secretRepo(e).MarkIsolated(e.ctx, scope, isolated, ""); err != nil || !ok {
		t.Fatalf("mark isolated: %v %v", ok, err)
	}

	for name, id := range map[string]string{"unpaired": unpaired, "missing": missing, "needs": needs, "notReady": notReady, "alreadyIsolated": isolated} {
		ok, err := repo.EnqueueIsolate(e.ctx, scope, id, "test")
		if err != nil || ok {
			t.Fatalf("%s: enqueued=%v err=%v", name, ok, err)
		}
	}
	// router soft-deleted
	if err := e.repo.SoftDelete(e.ctx, scope, r2.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got := f.pending(t); len(got) != 0 {
		t.Fatalf("no task expected, got %d", len(got))
	}
	// the eligible one works, with priority and event
	ok, err := repo.EnqueueIsolate(e.ctx, scope, f.secretID, "invoice.overdue")
	if err != nil || !ok {
		t.Fatalf("eligible: %v %v", ok, err)
	}
	got := f.pending(t)
	if len(got) != 1 || got[0].Action != domain.ActionIsolate || got[0].Priority != domain.PriorityIsolate ||
		got[0].CreatedByEvent != "invoice.overdue" || got[0].Status != domain.TaskPending ||
		got[0].RouterID != f.routerID || got[0].RouterName != "R1" || got[0].SecretName != "alice" || got[0].AccountName != "Cust" || got[0].AccountID == "" {
		t.Fatalf("unexpected view: %+v", got)
	}
}

func TestEnqueue_SecondCallIdempotent(t *testing.T) {
	f := newIsoFixture(t)
	repo := taskRepo(f.e)
	if ok, err := repo.EnqueueIsolate(f.e.ctx, f.scope, f.secretID, "a"); err != nil || !ok {
		t.Fatalf("first: %v %v", ok, err)
	}
	if ok, err := repo.EnqueueIsolate(f.e.ctx, f.scope, f.secretID, "b"); err != nil || ok {
		t.Fatalf("second: %v %v", ok, err)
	}
	if got := f.pending(t); len(got) != 1 {
		t.Fatalf("want 1 task, got %d", len(got))
	}
}

func TestEnqueueRestore_CancelsPendingIsolate(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, taskRepo(f.e)
	if ok, _ := repo.EnqueueIsolate(e.ctx, f.scope, f.secretID, "a"); !ok {
		t.Fatal("isolate not enqueued")
	}
	// A pending isolate while the secret is still normal: restore is not eligible.
	if ok, err := repo.EnqueueRestore(e.ctx, f.scope, f.secretID, "paid"); err != nil || ok {
		t.Fatalf("restore on normal secret: %v %v", ok, err)
	}
	if got := f.pending(t); len(got) != 1 || got[0].Status != domain.TaskPending {
		t.Fatalf("isolate must stay pending when restore is ineligible: %+v", got)
	}
	// Secret becomes isolated (as if the isolate ran), a stray pending isolate exists.
	if ok, _ := secretRepo(e).MarkIsolated(e.ctx, f.scope, f.secretID, ""); !ok {
		t.Fatal("mark isolated")
	}
	ok, err := repo.EnqueueRestore(e.ctx, f.scope, f.secretID, "paid")
	if err != nil || !ok {
		t.Fatalf("restore: %v %v", ok, err)
	}
	var cancelled, pending int
	for _, v := range f.pending(t) {
		switch {
		case v.Action == domain.ActionIsolate && v.Status == domain.TaskCancelled && v.FinishedAt != nil:
			cancelled++
		case v.Action == domain.ActionRestore && v.Status == domain.TaskPending && v.Priority == domain.PriorityRestore:
			pending++
		}
	}
	if cancelled != 1 || pending != 1 {
		t.Fatalf("cancelled=%d pending=%d", cancelled, pending)
	}
}

func TestEnqueueIsolate_CancelsPendingRestore(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, taskRepo(f.e)
	secretRepo(e).MarkIsolated(e.ctx, f.scope, f.secretID, "")
	if ok, _ := repo.EnqueueRestore(e.ctx, f.scope, f.secretID, "paid"); !ok {
		t.Fatal("restore not enqueued")
	}
	// secret flips back to normal while the restore is pending; new overdue arrives.
	secretRepo(e).MarkNormal(e.ctx, f.scope, f.secretID)
	ok, err := repo.EnqueueIsolate(e.ctx, f.scope, f.secretID, "overdue")
	if err != nil || !ok {
		t.Fatalf("isolate: %v %v", ok, err)
	}
	for _, v := range f.pending(t) {
		if v.Action == domain.ActionRestore && v.Status != domain.TaskCancelled {
			t.Fatalf("restore must be cancelled: %+v", v)
		}
	}
}

func TestEnqueueRestore_IgnoresIsolirReadyFalse(t *testing.T) {
	f := newIsoFixture(t)
	e := f.e
	secretRepo(e).MarkIsolated(e.ctx, f.scope, f.secretID, "")
	if err := e.repo.MarkSetup(e.ctx, f.scope, f.routerID, false, "", ""); err != nil {
		t.Fatal(err)
	}
	if ok, err := taskRepo(e).EnqueueRestore(e.ctx, f.scope, f.secretID, "paid"); err != nil || !ok {
		t.Fatalf("restore: %v %v", ok, err)
	}
}

func TestClaimBatch_PriorityOrderAndLease(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, taskRepo(f.e)
	s2 := pairedSecret(t, e, f.scope, f.routerID, "bob")
	secretRepo(e).MarkIsolated(e.ctx, f.scope, s2, "")
	repo.EnqueueIsolate(e.ctx, f.scope, f.secretID, "a")
	repo.EnqueueRestore(e.ctx, f.scope, s2, "b")

	now := time.Now().UTC()
	got, err := repo.ClaimBatch(e.ctx, f.scope, 1, now, time.Minute)
	if err != nil || len(got) != 1 || got[0].Action != domain.ActionRestore {
		t.Fatalf("restore first: %v %+v", err, got)
	}
	if got[0].Status != domain.TaskRunning || got[0].LockedUntil == nil || got[0].StartedAt == nil {
		t.Fatalf("claimed fields: %+v", got[0])
	}
	// Second claim: only the isolate remains (restore still leased).
	got2, _ := repo.ClaimBatch(e.ctx, f.scope, 10, now, time.Minute)
	if len(got2) != 1 || got2[0].Action != domain.ActionIsolate {
		t.Fatalf("second claim: %+v", got2)
	}
	// Nothing more before leases expire.
	if got3, _ := repo.ClaimBatch(e.ctx, f.scope, 10, now.Add(30*time.Second), time.Minute); len(got3) != 0 {
		t.Fatalf("unexpired lease reclaimed: %+v", got3)
	}
	// Expired leases are reclaimed.
	got4, _ := repo.ClaimBatch(e.ctx, f.scope, 10, now.Add(2*time.Minute), time.Minute)
	if len(got4) != 2 {
		t.Fatalf("expired lease must be reclaimed: %+v", got4)
	}
	// Future next_attempt_at is not claimable.
	if err := repo.Release(e.ctx, f.scope, got4[0].ID, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got5, _ := repo.ClaimBatch(e.ctx, f.scope, 10, now.Add(3*time.Minute), time.Minute); len(got5) != 0 {
		t.Fatalf("future task claimed: %+v", got5)
	}
}

func TestClaimBatch_ConcurrentNoDoubleClaim(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, taskRepo(f.e)
	for i := 0; i < 6; i++ {
		id := pairedSecret(t, e, f.scope, f.routerID, "u"+string(rune('a'+i)))
		if ok, err := repo.EnqueueIsolate(e.ctx, f.scope, id, "x"); err != nil || !ok {
			t.Fatalf("enqueue: %v %v", ok, err)
		}
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([][]domain.IsolationTask, 2)
	errs := make([]error, 2)
	now := time.Now().UTC()
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = repo.ClaimBatch(e.ctx, f.scope, 4, now, time.Minute)
		}(i)
	}
	close(start)
	wg.Wait()
	seen := map[string]bool{}
	total := 0
	for i := range results {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		for _, task := range results[i] {
			if seen[task.ID] {
				t.Fatalf("task %s claimed twice", task.ID)
			}
			seen[task.ID] = true
			total++
		}
	}
	if total != 6 {
		t.Fatalf("claimed %d, want 6", total)
	}
}

func TestMarkIsolated_RequiresPairing_ConcurrentUnpair(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, secretRepo(f.e)
	if err := repo.Unpair(e.ctx, f.scope, f.secretID); err != nil {
		t.Fatal(err)
	}
	ok, err := repo.MarkIsolated(e.ctx, f.scope, f.secretID, "paket")
	if err != nil || ok {
		t.Fatalf("unpaired must not isolate: %v %v", ok, err)
	}
	got, _ := repo.FindByID(e.ctx, f.scope, f.secretID)
	if got.IsolationState != domain.IsolationNormal {
		t.Fatalf("state changed: %+v", got)
	}
}

func TestMarkIsolated_StoresOriginalProfileOnlyWhenNonEmpty(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, secretRepo(f.e)
	ok, err := repo.MarkIsolated(e.ctx, f.scope, f.secretID, "")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if got, _ := repo.FindByID(e.ctx, f.scope, f.secretID); got.OriginalProfile != "paket" || got.IsolationState != domain.IsolationIsolated {
		t.Fatalf("empty keeps original: %+v", got)
	}
	// second call: no longer normal
	if ok, _ := repo.MarkIsolated(e.ctx, f.scope, f.secretID, "x"); ok {
		t.Fatal("second MarkIsolated must report false")
	}
	repo.MarkNormal(e.ctx, f.scope, f.secretID)
	if ok, _ := repo.MarkIsolated(e.ctx, f.scope, f.secretID, "gold"); !ok {
		t.Fatal("mark isolated with profile")
	}
	if got, _ := repo.FindByID(e.ctx, f.scope, f.secretID); got.OriginalProfile != "gold" {
		t.Fatalf("original_profile = %q", got.OriginalProfile)
	}
}

func TestMarkNormal_OnlyFromIsolated(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, secretRepo(f.e)
	if ok, err := repo.MarkNormal(e.ctx, f.scope, f.secretID); err != nil || ok {
		t.Fatalf("normal->normal: %v %v", ok, err)
	}
	repo.MarkIsolated(e.ctx, f.scope, f.secretID, "")
	if ok, err := repo.MarkNormal(e.ctx, f.scope, f.secretID); err != nil || !ok {
		t.Fatalf("isolated->normal: %v %v", ok, err)
	}
	if ok, _ := repo.MarkNormal(e.ctx, f.scope, f.secretID); ok {
		t.Fatal("second call must be false")
	}
}

func TestMarkAbsent(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, secretRepo(f.e)
	if err := repo.MarkAbsent(e.ctx, f.scope, f.secretID); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.FindByID(e.ctx, f.scope, f.secretID); got.Present {
		t.Fatal("still present")
	}
	if err := repo.MarkAbsent(e.ctx, f.scope, "00000000-0000-0000-0000-000000000001"); !errors.Is(err, mikrotik.ErrSecretNotFound) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestSecretRepo_ListByAccountAndIsolatedAccounts(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, secretRepo(f.e)
	s, _ := repo.FindByID(e.ctx, f.scope, f.secretID)
	acct := *s.AccountID
	// second secret on the same account
	s2 := seedSecret(t, e, f.scope, f.routerID, repository.ImportedSecret{Name: "alice2", Profile: "p"})
	if err := repo.Pair(e.ctx, f.scope, s2, acct, ""); err != nil {
		t.Fatal(err)
	}
	got, err := repo.ListByAccount(e.ctx, f.scope, acct)
	if err != nil || len(got) != 2 {
		t.Fatalf("list by account: %v %+v", err, got)
	}
	if ids, err := repo.AccountsWithIsolatedSecrets(e.ctx, f.scope); err != nil || len(ids) != 0 {
		t.Fatalf("none isolated yet: %v %v", err, ids)
	}
	repo.MarkIsolated(e.ctx, f.scope, f.secretID, "")
	repo.MarkIsolated(e.ctx, f.scope, s2, "")
	ids, err := repo.AccountsWithIsolatedSecrets(e.ctx, f.scope)
	if err != nil || len(ids) != 1 || ids[0] != acct {
		t.Fatalf("distinct accounts: %v %v", err, ids)
	}
	// other org sees nothing
	if other, _ := repo.ListByAccount(e.ctx, e.tenants.B.Scope, acct); len(other) != 0 {
		t.Fatalf("cross-org leak: %+v", other)
	}
}

func TestTaskRepo_Lifecycle(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, taskRepo(f.e)
	repo.EnqueueIsolate(e.ctx, f.scope, f.secretID, "x")
	now := time.Now().UTC()
	claimed, _ := repo.ClaimBatch(e.ctx, f.scope, 1, now, time.Minute)
	id := claimed[0].ID

	if err := repo.Release(e.ctx, f.scope, id, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.FindByID(e.ctx, f.scope, id)
	if got.Status != domain.TaskPending || got.Attempts != 0 || got.LockedUntil != nil {
		t.Fatalf("release: %+v", got)
	}

	claimed, _ = repo.ClaimBatch(e.ctx, f.scope, 1, now.Add(2*time.Minute), time.Minute)
	if len(claimed) != 1 {
		t.Fatal("reclaim")
	}
	next := now.Add(10 * time.Minute)
	if err := repo.RetryLater(e.ctx, f.scope, id, next, "unreachable", "boom"); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.FindByID(e.ctx, f.scope, id)
	if got.Status != domain.TaskPending || got.Attempts != 1 || got.LockedUntil != nil || got.LastErrorCode != "unreachable" ||
		got.LastError != "boom" || got.NextAttemptAt.Sub(next).Abs() > time.Second {
		t.Fatalf("retry later: %+v", got)
	}

	claimed, _ = repo.ClaimBatch(e.ctx, f.scope, 1, next.Add(time.Second), time.Minute)
	if err := repo.Fail(e.ctx, f.scope, id, "auth", "denied"); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.FindByID(e.ctx, f.scope, id)
	if got.Status != domain.TaskFailed || got.FinishedAt == nil || got.LockedUntil != nil || got.LastErrorCode != "auth" {
		t.Fatalf("fail: %+v", got)
	}

	if err := repo.MarkTenantNotified(e.ctx, f.scope, id); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkCustomerNotified(e.ctx, f.scope, id); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.FindByID(e.ctx, f.scope, id)
	if got.TenantNotifiedAt == nil || got.CustomerNotifiedAt == nil {
		t.Fatalf("notified: %+v", got)
	}

	// Retry: failed -> pending, attempts reset
	retried, err := repo.Retry(e.ctx, f.scope, id)
	if err != nil || retried.Status != domain.TaskPending || retried.Attempts != 0 || retried.FinishedAt != nil {
		t.Fatalf("retry: %v %+v", err, retried)
	}
	claimed, _ = repo.ClaimBatch(e.ctx, f.scope, 1, time.Now().UTC().Add(time.Second), time.Minute)
	if len(claimed) != 1 {
		t.Fatal("retried task must be claimable")
	}
	if err := repo.Complete(e.ctx, f.scope, id); err != nil {
		t.Fatal(err)
	}
	got, _ = repo.FindByID(e.ctx, f.scope, id)
	if got.Status != domain.TaskDone || got.FinishedAt == nil || got.LockedUntil != nil {
		t.Fatalf("complete: %+v", got)
	}
}

func TestRetry_OnlyFromFailed(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, taskRepo(f.e)
	repo.EnqueueIsolate(e.ctx, f.scope, f.secretID, "x")
	items := f.pending(t)
	if _, err := repo.Retry(e.ctx, f.scope, items[0].ID); !errors.Is(err, mikrotik.ErrTaskNotRetryable) {
		t.Fatalf("pending: %v", err)
	}
	if _, err := repo.Retry(e.ctx, f.scope, "00000000-0000-0000-0000-000000000001"); !errors.Is(err, mikrotik.ErrTaskNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := repo.Retry(e.ctx, e.tenants.B.Scope, items[0].ID); !errors.Is(err, mikrotik.ErrTaskNotFound) {
		t.Fatalf("cross-org: %v", err)
	}
	if _, err := repo.FindByID(e.ctx, e.tenants.B.Scope, items[0].ID); !errors.Is(err, mikrotik.ErrTaskNotFound) {
		t.Fatalf("find cross-org: %v", err)
	}
}

func TestTaskRepo_CancelListFiltersAndCounts(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, taskRepo(f.e)
	r2 := e.create(t, f.scope, "R2")
	e.repo.MarkSetup(e.ctx, f.scope, r2.ID, true, "isolir", "")
	s2 := pairedSecret(t, e, f.scope, r2.ID, "bob")
	s3 := pairedSecret(t, e, f.scope, r2.ID, "carol")
	for _, id := range []string{f.secretID, s2, s3} {
		if ok, _ := repo.EnqueueIsolate(e.ctx, f.scope, id, "x"); !ok {
			t.Fatal("enqueue")
		}
	}
	all := f.pending(t)
	var cancelID string
	for _, v := range all {
		if v.SecretName == "carol" {
			cancelID = v.ID
		}
	}
	if err := repo.Cancel(e.ctx, f.scope, cancelID, "manual"); err != nil {
		t.Fatal(err)
	}
	c, _ := repo.FindByID(e.ctx, f.scope, cancelID)
	if c.Status != domain.TaskCancelled || c.FinishedAt == nil || c.LastError != "manual" || c.LockedUntil != nil {
		t.Fatalf("cancel: %+v", c)
	}
	// A cancelled task cannot be cancelled again; unknown -> not found.
	if err := repo.Cancel(e.ctx, f.scope, "00000000-0000-0000-0000-000000000001", "x"); !errors.Is(err, mikrotik.ErrTaskNotFound) {
		t.Fatalf("cancel unknown: %v", err)
	}

	items, total, err := repo.List(e.ctx, f.scope, repository.TaskFilter{Status: domain.TaskPending}, 50, 0)
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("status filter: %v %d %d", err, total, len(items))
	}
	items, total, _ = repo.List(e.ctx, f.scope, repository.TaskFilter{RouterID: r2.ID}, 50, 0)
	if total != 2 || len(items) != 2 {
		t.Fatalf("router filter: %d %d", total, len(items))
	}
	items, total, _ = repo.List(e.ctx, f.scope, repository.TaskFilter{}, 1, 1)
	if total != 3 || len(items) != 1 {
		t.Fatalf("pagination: total=%d len=%d", total, len(items))
	}
	counts, err := repo.CountByStatus(e.ctx, f.scope)
	if err != nil || counts[domain.TaskPending] != 2 || counts[domain.TaskCancelled] != 1 {
		t.Fatalf("counts: %v %v", err, counts)
	}
	if other, _ := repo.CountByStatus(e.ctx, e.tenants.B.Scope); len(other) != 0 {
		t.Fatalf("cross-org counts: %v", other)
	}
}

func TestRetry_ActiveTaskExistsMapsToDomainError(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, taskRepo(f.e)
	if ok, err := repo.EnqueueIsolate(e.ctx, f.scope, f.secretID, "x"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	now := time.Now().UTC()
	claimed, err := repo.ClaimBatch(e.ctx, f.scope, 1, now, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatal(err, claimed)
	}
	failedID := claimed[0].ID
	if err := repo.Fail(e.ctx, f.scope, failedID, "auth", "denied"); err != nil {
		t.Fatal(err)
	}
	if ok, err := secretRepo(e).MarkIsolated(e.ctx, f.scope, f.secretID, ""); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := repo.EnqueueRestore(e.ctx, f.scope, f.secretID, "paid"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, err := repo.Retry(e.ctx, f.scope, failedID); !errors.Is(err, mikrotik.ErrTaskActiveExists) {
		t.Fatalf("retry: %v", err)
	}
	got, err := repo.FindByID(e.ctx, f.scope, failedID)
	if err != nil || got.Status != domain.TaskFailed {
		t.Fatalf("must stay failed: %v %+v", err, got)
	}
}

func TestEnqueueRestore_RunningIsolateUntouched(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, taskRepo(f.e)
	if ok, err := repo.EnqueueIsolate(e.ctx, f.scope, f.secretID, "x"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	claimed, err := repo.ClaimBatch(e.ctx, f.scope, 1, time.Now().UTC(), time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatal(err, claimed)
	}
	if ok, err := secretRepo(e).MarkIsolated(e.ctx, f.scope, f.secretID, ""); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if ok, err := repo.EnqueueRestore(e.ctx, f.scope, f.secretID, "paid"); err != nil || ok {
		t.Fatalf("restore vs running isolate: %v %v", ok, err)
	}
	got, err := repo.FindByID(e.ctx, f.scope, claimed[0].ID)
	if err != nil || got.Status != domain.TaskRunning || got.LockedUntil == nil {
		t.Fatalf("running isolate touched: %v %+v", err, got)
	}
	if n := len(f.pending(t)); n != 1 {
		t.Fatalf("tasks = %d", n)
	}
}

func TestTaskRepo_LifecycleOnTerminalTaskIsNotFound(t *testing.T) {
	f := newIsoFixture(t)
	e, repo := f.e, taskRepo(f.e)
	if ok, err := repo.EnqueueIsolate(e.ctx, f.scope, f.secretID, "x"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	now := time.Now().UTC()
	claimed, err := repo.ClaimBatch(e.ctx, f.scope, 1, now, time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatal(err, claimed)
	}
	id := claimed[0].ID
	if err := repo.Complete(e.ctx, f.scope, id); err != nil {
		t.Fatal(err)
	}
	check := func(label string) {
		t.Helper()
		for name, err := range map[string]error{
			"Complete":   repo.Complete(e.ctx, f.scope, id),
			"Fail":       repo.Fail(e.ctx, f.scope, id, "c", "m"),
			"Release":    repo.Release(e.ctx, f.scope, id, now),
			"RetryLater": repo.RetryLater(e.ctx, f.scope, id, now, "c", "m"),
		} {
			if !errors.Is(err, mikrotik.ErrTaskNotFound) {
				t.Fatalf("%s %s: %v", label, name, err)
			}
		}
		if got, _ := repo.FindByID(e.ctx, f.scope, id); got.Status != label {
			t.Fatalf("%s status changed to %s", label, got.Status)
		}
	}
	check(domain.TaskDone)
	// Cancel on a finished task is a no-op, twice.
	for i := 0; i < 2; i++ {
		if err := repo.Cancel(e.ctx, f.scope, id, "late"); err != nil {
			t.Fatalf("cancel finished #%d: %v", i, err)
		}
	}
	if got, _ := repo.FindByID(e.ctx, f.scope, id); got.Status != domain.TaskDone || got.LastError != "" {
		t.Fatalf("finished task modified: %+v", got)
	}

	// Cancel a running task, then Complete is refused.
	if err := secretRepo(e).MarkAbsent(e.ctx, f.scope, f.secretID); err != nil {
		t.Fatal(err)
	}
	s2 := pairedSecret(t, e, f.scope, f.routerID, "bob")
	if ok, err := repo.EnqueueIsolate(e.ctx, f.scope, s2, "x"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	claimed, err = repo.ClaimBatch(e.ctx, f.scope, 1, now.Add(time.Second), time.Minute)
	if err != nil || len(claimed) != 1 {
		t.Fatal(err, claimed)
	}
	id = claimed[0].ID
	if err := repo.Cancel(e.ctx, f.scope, id, "operator"); err != nil {
		t.Fatal(err)
	}
	check(domain.TaskCancelled)
}
