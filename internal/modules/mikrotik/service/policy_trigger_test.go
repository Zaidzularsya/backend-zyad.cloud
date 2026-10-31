package service

import (
	"context"
	"errors"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
)

// UpsertProductTrigger mirrors the SQL semantics (CASE on the old enabled value).
func (f *fakePolicyRepo) UpsertProductTrigger(_ context.Context, scope coretenant.Scope, productID string, enabled bool, today time.Time, by string) (domain.ProductTrigger, error) {
	f.trCalls++
	if f.triggers == nil {
		f.triggers = map[string]domain.ProductTrigger{}
	}
	old, ok := f.triggers[productID]
	t := domain.ProductTrigger{OrganizationID: scope.OrganizationID(), ProductID: productID, Enabled: enabled, EffectiveFrom: today}
	if ok && !(enabled && !old.Enabled) {
		t.EffectiveFrom = old.EffectiveFrom
	}
	if by != "" {
		t.SetBy = &by
	}
	f.triggers[productID] = t
	return t, nil
}

func (f *fakePolicyRepo) GetProductTrigger(_ context.Context, _ coretenant.Scope, productID string) (domain.ProductTrigger, bool, error) {
	t, ok := f.triggers[productID]
	return t, ok, nil
}

func (f *fakePolicyRepo) ListProductTriggers(context.Context, coretenant.Scope) ([]domain.ProductTrigger, error) {
	var out []domain.ProductTrigger
	for _, t := range f.triggers {
		out = append(out, t)
	}
	return out, nil
}

type fakeCatalog struct {
	products map[string]ProductInfo
	inUse    []ProductInfo
}

func (f fakeCatalog) Get(_ context.Context, _ coretenant.Scope, id string) (ProductInfo, error) {
	if p, ok := f.products[id]; ok {
		return p, nil
	}
	return ProductInfo{}, mikrotik.ErrProductNotFound
}

func (f fakeCatalog) ListRecurringInUse(context.Context, coretenant.Scope) ([]ProductInfo, error) {
	return f.inUse, nil
}

type fakeUsage struct {
	got   time.Time
	usage ProductUsage
}

func (f *fakeUsage) ForProduct(_ context.Context, _ coretenant.Scope, _ string, from time.Time) (ProductUsage, error) {
	f.got = from
	return f.usage, nil
}

type triggerEnv struct {
	*policyEnv
	now   time.Time
	usage *fakeUsage
}

func newTriggerEnv(t *testing.T) *triggerEnv {
	t.Helper()
	e := &triggerEnv{policyEnv: &policyEnv{repo: &fakePolicyRepo{}, rrepo: &fakeRepo{}, scope: testScope(t)}}
	e.now = time.Date(2026, 11, 5, 18, 0, 0, 0, time.UTC) // 6 Nov 01:00 WIB
	e.usage = &fakeUsage{usage: ProductUsage{ActiveContracts: 3, Accounts: 2, OverdueInvoices: 1}}
	pppoe := ProductInfo{ID: "p-1", Name: "PPPoE 10M", SKU: "P10", Recurring: true, Active: true}
	other := ProductInfo{ID: "p-2", Name: "Hosting", Recurring: true, Active: true}
	e.svc = NewPolicyService(PolicyDeps{
		Repo: e.repo, Audit: e.rrepo, Usage: e.usage,
		Accounts: fakeAccounts{},
		Products: fakeCatalog{products: map[string]ProductInfo{"p-1": pppoe, "p-2": other}, inUse: []ProductInfo{pppoe, other}},
		Now:      func() time.Time { return e.now },
	})
	return e
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestSetTrigger_EffectiveFromIsTodayWIB(t *testing.T) {
	e := newTriggerEnv(t)
	tr, err := e.svc.SetTrigger(context.Background(), e.scope, "p-1", true, testActor)
	if err != nil || !tr.Enabled || !tr.EffectiveFrom.Equal(day(2026, 11, 6)) {
		t.Fatalf("trigger = %+v, %v", tr, err)
	}
	if len(e.rrepo.audits) != 1 {
		t.Fatalf("audits = %d", len(e.rrepo.audits))
	}
	a := e.rrepo.audits[0]
	if a.Event != "product_trigger.set" || a.TargetID != "p-1" || a.Metadata["target"] != "product_trigger" ||
		a.Metadata["product_id"] != "p-1" || a.Metadata["enabled"] != true || a.Metadata["effective_from"] != "2026-11-06" {
		t.Fatalf("audit = %+v", a)
	}
}

func TestSetTrigger_IdempotentKeepsEffectiveFrom(t *testing.T) {
	e := newTriggerEnv(t)
	if _, err := e.svc.SetTrigger(context.Background(), e.scope, "p-1", true, testActor); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.AddDate(0, 0, 10)
	tr, err := e.svc.SetTrigger(context.Background(), e.scope, "p-1", true, testActor)
	if err != nil || !tr.EffectiveFrom.Equal(day(2026, 11, 6)) {
		t.Fatalf("trigger = %+v, %v", tr, err)
	}
}

func TestSetTrigger_DisableThenEnableResetsDate(t *testing.T) {
	e := newTriggerEnv(t)
	ctx := context.Background()
	if _, err := e.svc.SetTrigger(ctx, e.scope, "p-1", true, testActor); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.AddDate(0, 0, 10) // 16 Nov WIB
	tr, err := e.svc.SetTrigger(ctx, e.scope, "p-1", false, testActor)
	if err != nil || tr.Enabled || !tr.EffectiveFrom.Equal(day(2026, 11, 6)) {
		t.Fatalf("disable = %+v, %v", tr, err)
	}
	if len(e.repo.triggers) != 1 {
		t.Fatal("disable must keep the row")
	}
	e.now = e.now.AddDate(0, 0, 5) // 21 Nov WIB
	tr, err = e.svc.SetTrigger(ctx, e.scope, "p-1", true, testActor)
	if err != nil || !tr.Enabled || !tr.EffectiveFrom.Equal(day(2026, 11, 21)) {
		t.Fatalf("re-enable = %+v, %v", tr, err)
	}
}

func TestSetTrigger_ProductOfOtherOrg404(t *testing.T) {
	e := newTriggerEnv(t)
	_, err := e.svc.SetTrigger(context.Background(), e.scope, "p-other", true, testActor)
	if !errors.Is(err, mikrotik.ErrProductNotFound) {
		t.Fatalf("err = %v", err)
	}
	if e.repo.trCalls != 0 || len(e.rrepo.audits) != 0 {
		t.Fatal("must not reach repo or audit")
	}
	if _, err := e.svc.PreviewTrigger(context.Background(), e.scope, "p-other"); !errors.Is(err, mikrotik.ErrProductNotFound) {
		t.Fatalf("preview err = %v", err)
	}
}

func TestPreviewTrigger_ReturnsUsage(t *testing.T) {
	e := newTriggerEnv(t)
	ctx := context.Background()
	pv, err := e.svc.PreviewTrigger(ctx, e.scope, "p-1")
	if err != nil || pv.Product.ID != "p-1" || pv.Usage.OverdueInvoices != 1 || pv.Usage.ActiveContracts != 3 ||
		pv.AlreadyEnabled || !pv.EffectiveFrom.Equal(day(2026, 11, 6)) || !e.usage.got.Equal(day(2026, 11, 6)) {
		t.Fatalf("preview = %+v, %v", pv, err)
	}
	if e.repo.trCalls != 0 {
		t.Fatal("preview must not write")
	}
	// Already enabled: reports the existing date, not today.
	if _, err := e.svc.SetTrigger(ctx, e.scope, "p-1", true, testActor); err != nil {
		t.Fatal(err)
	}
	e.now = e.now.AddDate(0, 0, 3)
	pv, err = e.svc.PreviewTrigger(ctx, e.scope, "p-1")
	if err != nil || !pv.AlreadyEnabled || !pv.EffectiveFrom.Equal(day(2026, 11, 6)) || !e.usage.got.Equal(day(2026, 11, 6)) {
		t.Fatalf("preview enabled = %+v, %v", pv, err)
	}
}

func TestUnmarkedProducts_ExcludesEnabled(t *testing.T) {
	e := newTriggerEnv(t)
	ctx := context.Background()
	if _, err := e.svc.SetTrigger(ctx, e.scope, "p-1", true, testActor); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.UnmarkedProducts(ctx, e.scope)
	if err != nil || len(got) != 1 || got[0].ID != "p-2" {
		t.Fatalf("unmarked = %+v, %v", got, err)
	}
	// A disabled trigger counts as unmarked again.
	if _, err := e.svc.SetTrigger(ctx, e.scope, "p-1", false, testActor); err != nil {
		t.Fatal(err)
	}
	got, _ = e.svc.UnmarkedProducts(ctx, e.scope)
	if len(got) != 2 {
		t.Fatalf("unmarked after disable = %+v", got)
	}
	list, err := e.svc.ListTriggers(ctx, e.scope)
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
}

func TestListTriggersWithProducts(t *testing.T) {
	e := newTriggerEnv(t)
	ctx := context.Background()
	if _, err := e.svc.SetTrigger(ctx, e.scope, "p-1", true, testActor); err != nil {
		t.Fatal(err)
	}
	items, err := e.svc.ListTriggersWithProducts(ctx, e.scope)
	if err != nil || len(items) != 1 || items[0].Product == nil || items[0].Product.ID != "p-1" || items[0].Product.Name == "" {
		t.Fatalf("items = %+v, %v", items, err)
	}
}
