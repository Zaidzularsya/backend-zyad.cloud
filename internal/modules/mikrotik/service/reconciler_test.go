package service

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

// recSource serves AccountsWithTriggerInvoices from a sorted list with keyset paging.
type recSource struct {
	accounts []string
	pages    int
	limits   []int
}

func (s *recSource) TriggerInvoices(context.Context, coretenant.Scope, string, time.Time) ([]TriggerInvoice, error) {
	return oneInvoice, nil
}

func (s *recSource) AccountsWithTriggerInvoices(_ context.Context, _ coretenant.Scope, _ time.Time, after string, limit int) ([]string, error) {
	s.pages++
	s.limits = append(s.limits, limit)
	var out []string
	for _, a := range s.accounts {
		if a > after && len(out) < limit {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s *recSource) AccountOfInvoice(context.Context, coretenant.Scope, string) (string, error) {
	return "", nil
}

// recSecrets records which accounts the evaluator looked at.
type recSecrets struct {
	repository.SecretRepository
	mu       sync.Mutex
	isolated []string
	failFor  string
	seen     []string
}

func (s *recSecrets) AccountsWithIsolatedSecrets(context.Context, coretenant.Scope) ([]string, error) {
	return s.isolated, nil
}

func (s *recSecrets) ListByAccount(_ context.Context, _ coretenant.Scope, accountID string) ([]domain.PPPSecret, error) {
	s.mu.Lock()
	s.seen = append(s.seen, accountID)
	s.mu.Unlock()
	if accountID == s.failFor {
		return nil, errBoom
	}
	return nil, nil
}

func newRecEnv(t *testing.T, triggers, isolated []string) (*Reconciler, *recSource, *recSecrets) {
	t.Helper()
	src := &recSource{accounts: triggers}
	sec := &recSecrets{isolated: isolated}
	now := func() time.Time { return time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC) }
	eval := NewEvaluator(EvaluatorDeps{Source: src, Policy: &fakePolicyRepo{}, Secrets: sec, Tasks: &evalTasks{}, Mode: ModeOn, Now: now})
	return NewReconciler(src, sec, eval, now), src, sec
}

func TestReconciler_UnionOfTriggerAndIsolatedAccounts(t *testing.T) {
	rec, _, sec := newRecEnv(t, []string{"a", "b"}, []string{"b", "c"})
	res, err := rec.RunOrganization(context.Background(), testScope(t))
	if err != nil {
		t.Fatal(err)
	}
	got := append([]string(nil), sec.seen...)
	sort.Strings(got)
	if fmt.Sprint(got) != "[a b c]" {
		t.Fatalf("evaluated %v, want each of a,b,c exactly once", got)
	}
	if res.Accounts != 3 || res.Failed != 0 {
		t.Fatalf("result %+v", res)
	}
}

func TestReconciler_OneAccountErrorContinues(t *testing.T) {
	rec, _, sec := newRecEnv(t, []string{"a", "b", "c"}, nil)
	sec.failFor = "b"
	res, err := rec.RunOrganization(context.Background(), testScope(t))
	if err != nil {
		t.Fatalf("per-account error must not fail the run: %v", err)
	}
	if len(sec.seen) != 3 {
		t.Fatalf("evaluated %v, want all three", sec.seen)
	}
	if res.Accounts != 3 || res.Failed != 1 {
		t.Fatalf("result %+v", res)
	}
}

func TestReconciler_PagesThroughAllTriggerAccounts(t *testing.T) {
	var triggers []string
	for i := 0; i < 450; i++ {
		triggers = append(triggers, fmt.Sprintf("acc-%04d", i))
	}
	rec, src, sec := newRecEnv(t, triggers, nil)
	res, err := rec.RunOrganization(context.Background(), testScope(t))
	if err != nil {
		t.Fatal(err)
	}
	if res.Accounts != 450 || len(sec.seen) != 450 {
		t.Fatalf("accounts=%d seen=%d, want 450", res.Accounts, len(sec.seen))
	}
	if src.pages != 3 {
		t.Fatalf("pages=%d, want 3 (200+200+50)", src.pages)
	}
	for _, l := range src.limits {
		if l != reconcilePageSize {
			t.Fatalf("page limit %d, want %d", l, reconcilePageSize)
		}
	}
}

func TestReconciler_ContextCancelledStops(t *testing.T) {
	rec, _, sec := newRecEnv(t, []string{"a", "b"}, []string{"c"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := rec.RunOrganization(ctx, testScope(t)); err == nil {
		t.Fatal("want ctx error")
	}
	if len(sec.seen) != 0 {
		t.Fatalf("evaluated %v after cancel", sec.seen)
	}
}
