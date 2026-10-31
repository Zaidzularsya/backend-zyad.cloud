package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

type fakePolicyRepo struct {
	rows  map[string]domain.CustomerPolicy
	calls int
}

func (f *fakePolicyRepo) UpsertCustomerPolicy(_ context.Context, scope coretenant.Scope, accountID string, exempt bool, reason, by string) (domain.CustomerPolicy, error) {
	f.calls++
	if f.rows == nil {
		f.rows = map[string]domain.CustomerPolicy{}
	}
	p := domain.CustomerPolicy{OrganizationID: scope.OrganizationID(), AccountID: accountID, Exempt: exempt, Reason: reason}
	if by != "" {
		p.SetBy = &by
	}
	f.rows[accountID] = p
	return p, nil
}

func (f *fakePolicyRepo) GetCustomerPolicy(_ context.Context, _ coretenant.Scope, accountID string) (domain.CustomerPolicy, bool, error) {
	p, ok := f.rows[accountID]
	return p, ok, nil
}

type policyEnv struct {
	svc     *PolicyService
	repo    *fakePolicyRepo
	rrepo   *fakeRepo
	scope   coretenant.Scope
	changed []string
}

func newPolicyEnv(t *testing.T) *policyEnv {
	t.Helper()
	e := &policyEnv{repo: &fakePolicyRepo{}, rrepo: &fakeRepo{}, scope: testScope(t)}
	accounts := fakeAccounts{known: map[string]AccountRef{"acct-1": {ID: "acct-1", Name: "Alice"}}}
	e.svc = NewPolicyService(e.repo, accounts, e.rrepo, func(_ context.Context, _ coretenant.Scope, id string) { e.changed = append(e.changed, id) })
	return e
}

func TestSetExempt_RequiresReason(t *testing.T) {
	e := newPolicyEnv(t)
	for _, reason := range []string{"", "   ", strings.Repeat("x", 501)} {
		_, err := e.svc.SetExempt(context.Background(), e.scope, "acct-1", true, reason, testActor)
		var inv *mikrotik.InvalidInputError
		if !errors.As(err, &inv) {
			t.Fatalf("reason %d chars: err = %v", len(reason), err)
		}
	}
	if e.repo.calls != 0 || len(e.changed) != 0 {
		t.Fatal("must not write or notify on invalid input")
	}
	// 500 multi-byte runes is allowed (count runes, not bytes).
	if _, err := e.svc.SetExempt(context.Background(), e.scope, "acct-1", true, strings.Repeat("é", 500), testActor); err != nil {
		t.Fatalf("500 runes: %v", err)
	}
	// Reason optional when not exempt, stored when given.
	p, err := e.svc.SetExempt(context.Background(), e.scope, "acct-1", false, " selesai ", testActor)
	if err != nil || p.Exempt || p.Reason != "selesai" {
		t.Fatalf("unexempt = %+v, %v", p, err)
	}
	if _, err := e.svc.SetExempt(context.Background(), e.scope, "acct-1", false, "", testActor); err != nil {
		t.Fatalf("unexempt no reason: %v", err)
	}
}

func TestSetExempt_AccountOfOtherOrg404(t *testing.T) {
	e := newPolicyEnv(t)
	_, err := e.svc.SetExempt(context.Background(), e.scope, "acct-other", true, "alasan", testActor)
	if !errors.Is(err, mikrotik.ErrAccountNotFound) {
		t.Fatalf("err = %v", err)
	}
	if e.repo.calls != 0 || len(e.changed) != 0 || len(e.rrepo.audits) != 0 {
		t.Fatal("must not reach repo, audit or onChange")
	}
}

func TestSetExempt_UpsertIdempotent(t *testing.T) {
	e := newPolicyEnv(t)
	for i := 0; i < 2; i++ {
		if _, err := e.svc.SetExempt(context.Background(), e.scope, "acct-1", true, "VIP", testActor); err != nil {
			t.Fatal(err)
		}
	}
	if len(e.repo.rows) != 1 || !e.repo.rows["acct-1"].Exempt {
		t.Fatalf("rows = %+v", e.repo.rows)
	}
}

func TestSetExempt_WritesAudit(t *testing.T) {
	e := newPolicyEnv(t)
	if _, err := e.svc.SetExempt(context.Background(), e.scope, "acct-1", true, "VIP", testActor); err != nil {
		t.Fatal(err)
	}
	if len(e.rrepo.audits) != 1 {
		t.Fatalf("audits = %d", len(e.rrepo.audits))
	}
	a := e.rrepo.audits[0]
	if a.Event != "customer_policy.exempt_set" || a.ActorUserID != testActor || a.TargetID != "acct-1" {
		t.Fatalf("audit = %+v", a)
	}
	m := a.Metadata
	if m["target"] != "customer_policy" || m["account_id"] != "acct-1" || m["exempt"] != true || m["reason"] != "VIP" {
		t.Fatalf("meta = %+v", m)
	}
}

func TestGetPolicy_DefaultNotExempt(t *testing.T) {
	e := newPolicyEnv(t)
	p, err := e.svc.GetPolicy(context.Background(), e.scope, "acct-1")
	if err != nil || p.Exempt || p.AccountID != "acct-1" {
		t.Fatalf("p = %+v, %v", p, err)
	}
	if _, err := e.svc.GetPolicy(context.Background(), e.scope, "acct-other"); !errors.Is(err, mikrotik.ErrAccountNotFound) {
		t.Fatalf("other org err = %v", err)
	}
	_, _ = e.svc.SetExempt(context.Background(), e.scope, "acct-1", true, "VIP", testActor)
	if p, _ := e.svc.GetPolicy(context.Background(), e.scope, "acct-1"); !p.Exempt {
		t.Fatalf("p = %+v", p)
	}
}

func TestSetExempt_CallsOnChange(t *testing.T) {
	e := newPolicyEnv(t)
	if _, err := e.svc.SetExempt(context.Background(), e.scope, "acct-1", true, "VIP", testActor); err != nil {
		t.Fatal(err)
	}
	if len(e.changed) != 1 || e.changed[0] != "acct-1" {
		t.Fatalf("changed = %v", e.changed)
	}
	// nil onChange is a no-op.
	svc := NewPolicyService(e.repo, fakeAccounts{known: map[string]AccountRef{"acct-1": {ID: "acct-1"}}}, e.rrepo, nil)
	if _, err := svc.SetExempt(context.Background(), e.scope, "acct-1", false, "", testActor); err != nil {
		t.Fatal(err)
	}
}
