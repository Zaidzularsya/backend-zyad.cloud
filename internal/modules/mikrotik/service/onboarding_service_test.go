package service

import (
	"context"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/repository"
)

type fakeOnboardingRepo struct {
	counts   repository.OnboardingCounts
	statuses map[string]repository.AccountStatus
	err      error
}

func (f *fakeOnboardingRepo) Counts(context.Context, coretenant.Scope) (repository.OnboardingCounts, error) {
	return f.counts, f.err
}
func (f *fakeOnboardingRepo) AccountStatuses(context.Context, coretenant.Scope, []string) (map[string]repository.AccountStatus, error) {
	return f.statuses, f.err
}

type fakeSender struct {
	ok  bool
	err error
}

func (f fakeSender) HasDefaultSender(context.Context, coretenant.Scope) (bool, error) {
	return f.ok, f.err
}

func TestOnboarding_CompleteOnlyWhenAllFive(t *testing.T) {
	good := repository.OnboardingCounts{Routers: 1, RoutersReady: 1, SecretsTotal: 3, SecretsPaired: 2, NeedsProfile: 0, ProductsMarked: 1}
	cases := []struct {
		name   string
		mutate func(*repository.OnboardingCounts)
		sender bool
		want   bool
	}{
		{"all satisfied", func(*repository.OnboardingCounts) {}, true, true},
		{"router not ready", func(c *repository.OnboardingCounts) { c.RoutersReady = 0 }, true, false},
		{"no router at all", func(c *repository.OnboardingCounts) { c.Routers, c.RoutersReady = 0, 0 }, true, false},
		{"no paired secret", func(c *repository.OnboardingCounts) { c.SecretsPaired = 0 }, true, false},
		{"secret needs profile", func(c *repository.OnboardingCounts) { c.NeedsProfile = 1 }, true, false},
		{"no product marked", func(c *repository.OnboardingCounts) { c.ProductsMarked = 0 }, true, false},
		{"no sender", func(*repository.OnboardingCounts) {}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := good
			tc.mutate(&c)
			svc := NewOnboardingService(&fakeOnboardingRepo{counts: c}, fakeSender{ok: tc.sender})
			got, err := svc.Summary(context.Background(), coretenant.Scope{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Complete != tc.want {
				t.Fatalf("complete = %v, want %v (%+v)", got.Complete, tc.want, got)
			}
			if got.RouterRegistered != (c.Routers > 0) || got.SecretsPaired != c.SecretsPaired || got.SenderConfigured != tc.sender {
				t.Fatalf("fields not mapped: %+v", got)
			}
		})
	}
}

func TestOnboarding_SenderErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	svc := NewOnboardingService(&fakeOnboardingRepo{}, fakeSender{err: boom})
	if _, err := svc.Summary(context.Background(), coretenant.Scope{}); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestCustomerStatuses_NoneIsolatedNormal(t *testing.T) {
	repo := &fakeOnboardingRepo{statuses: map[string]repository.AccountStatus{
		"iso":    {Secrets: 2, Isolated: true},
		"norm":   {Secrets: 1, Exempt: true},
		"exonly": {Exempt: true, ExemptedUntil: "2026-12-01"},
	}}
	svc := NewOnboardingService(repo, fakeSender{})
	got, err := svc.CustomerStatuses(context.Background(), coretenant.Scope{}, []string{"norm", "none", "iso", "exonly"})
	if err != nil {
		t.Fatal(err)
	}
	want := []CustomerStatus{
		{AccountID: "norm", IsolationState: "normal", Exempt: true, Secrets: 1},
		{AccountID: "none", IsolationState: "none"},
		{AccountID: "iso", IsolationState: "isolated", Secrets: 2},
		{AccountID: "exonly", IsolationState: "none", Exempt: true, ExemptedUntil: "2026-12-01"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
