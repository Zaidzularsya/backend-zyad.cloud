package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/catalog/repository"
	"zyad.cloud/internal/shared/pricing"
)

type fakePublicRepo struct {
	repository.ProductRepository
	products []domain.Product
}

func (f fakePublicRepo) ListPublic(context.Context, coretenant.Scope) ([]domain.Product, error) {
	return f.products, nil
}

type fakeScopes struct{ err error }

func (f fakeScopes) PlatformScope(context.Context) (coretenant.Scope, error) {
	return coretenant.Scope{}, f.err
}

func pub(id, cat string, catPos int, code string, order int, freq pricing.Frequency, price string, feats ...domain.ProductFeature) domain.Product {
	c := cat
	attrs := pricing.Attributes{ChargeType: pricing.Recurring, Frequency: freq, PaymentTiming: pricing.Prepaid}
	return domain.Product{ID: id, SKU: "SKU-" + id, Name: "N-" + id, Description: "D-" + id, CategoryID: &c, CategoryName: cat,
		CategoryPosition: catPos, IsPublic: true, ListingCode: code, ListingOrder: order, BasePrice: price, TaxPercent: "11.00",
		Currency: "IDR", Pricing: attrs, IsActive: true, Features: feats}
}

func boolFeat(key string, v bool) domain.ProductFeature {
	b, _ := json.Marshal(v)
	return domain.ProductFeature{FeatureKey: key, Value: b}
}

func TestPublicListingsGroupsAndOrders(t *testing.T) {
	crm := boolFeat("crm", true)
	off := boolFeat("wa", false)
	users := domain.ProductFeature{FeatureKey: "users", Value: json.RawMessage(`5`), DisplayLabel: "Hingga 5 user"}
	repo := fakePublicRepo{products: []domain.Product{
		pub("y", "Zyad Cloud", 0, "freelancer", 2, pricing.Annual, "1500000.00", crm),
		pub("m", "Zyad Cloud", 0, "freelancer", 2, pricing.Monthly, "150000.00", users, off, crm),
		pub("t", "Zyad Cloud", 0, "team", 1, pricing.Monthly, "300000.00", crm),
		pub("o", "Add-on", 1, "extra", 0, pricing.Monthly, "10000.00"),
	}}
	svc := NewPublicListingService(repo, newFakeRegistry(), fakeScopes{})
	got, err := svc.Listings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Categories) != 2 || got.Categories[0].Name != "Zyad Cloud" || got.Categories[1].Name != "Add-on" {
		t.Fatalf("categories = %+v", got.Categories)
	}
	cards := got.Categories[0].Listings
	if len(cards) != 2 || cards[0].Code != "team" || cards[1].Code != "freelancer" {
		t.Fatalf("cards = %+v", cards)
	}
	f := cards[1]
	if len(f.Variants) != 2 || f.Variants[0].BillingFrequency != "monthly" || f.Variants[1].BillingFrequency != "annual" {
		t.Fatalf("variants = %+v", f.Variants)
	}
	if f.Name != "N-m" || f.Description != "D-m" {
		t.Fatalf("card harus memakai varian frekuensi terkecil: %+v", f)
	}
	if len(f.Benefits) != 2 || f.Benefits[0].Label != "Hingga 5 user" || f.Benefits[1].Label != "CRM" {
		t.Fatalf("benefits = %+v", f.Benefits)
	}
	if v := f.Variants[0]; v.PriceWithTax != "166500.00" || !v.CheckoutEnabled || v.ProductID != "m" {
		t.Fatalf("variant = %+v", v)
	}
}

func TestPublicListingsCheckoutEnabled(t *testing.T) {
	crm := boolFeat("crm", true)
	oneTime := pub("a", "C", 0, "a", 0, pricing.Monthly, "100.00", crm)
	oneTime.Pricing = pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Prepaid}
	free := pub("b", "C", 0, "b", 0, pricing.Monthly, "0.00", crm)
	noFeat := pub("c", "C", 0, "c", 0, pricing.Monthly, "100.00")
	postpaid := pub("d", "C", 0, "d", 0, pricing.Monthly, "100.00", crm)
	postpaid.Pricing.PaymentTiming = pricing.Postpaid
	svc := NewPublicListingService(fakePublicRepo{products: []domain.Product{oneTime, free, noFeat, postpaid}}, newFakeRegistry(), fakeScopes{})
	got, err := svc.Listings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range got.Categories[0].Listings {
		if l.Variants[0].CheckoutEnabled {
			t.Errorf("%s: checkout_enabled harus false", l.Code)
		}
	}
}

func TestPublicListingsPlatformMissingIsEmpty(t *testing.T) {
	svc := NewPublicListingService(fakePublicRepo{}, newFakeRegistry(), fakeScopes{err: ErrPlatformNotFound})
	got, err := svc.Listings(context.Background())
	if err != nil || got.Categories == nil || len(got.Categories) != 0 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	other := errors.New("db down")
	if _, err := NewPublicListingService(fakePublicRepo{}, newFakeRegistry(), fakeScopes{err: other}).Listings(context.Background()); !errors.Is(err, other) {
		t.Fatalf("error lain harus diteruskan: %v", err)
	}
}

func TestExcludeNeedsUpdate(t *testing.T) {
	reg := newFakeRegistry()
	good := pub("good", "Paket", 1, "a", 1, pricing.Monthly, "100", boolFeat("tcrm", true))
	bad := pub("bad", "Paket", 1, "a", 1, pricing.Quarterly, "100", boolFeat("wa", true)) // wa tidak tenant_offerable
	got, err := ExcludeNeedsUpdate(context.Background(), reg, []domain.Product{good, bad})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "good" {
		t.Fatalf("got %+v", got)
	}
}

// --- listing tenant (Task 8) ---

type orgPublicRepo struct {
	repository.ProductRepository
	byOrg map[string][]domain.Product
}

func (f orgPublicRepo) ListPublic(_ context.Context, s coretenant.Scope) ([]domain.Product, error) {
	return append([]domain.Product(nil), f.byOrg[s.OrganizationID()]...), nil
}

type fakeEntitlements struct{ granted map[string]bool }

func (f fakeEntitlements) RequireFeature(_ context.Context, _ string, key string) error {
	if f.granted[key] {
		return nil
	}
	return errors.New("not entitled")
}

func tenantScope(t *testing.T, orgID string) coretenant.Scope {
	t.Helper()
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: orgID, OrganizationSlug: "org-" + orgID[:4], OrganizationType: coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive, MembershipID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		MembershipStatus: "active", MembershipVersion: 1, ResolutionSource: coretenant.ResolutionSourceCustomDomain,
		DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := coretenant.NewScope(tc)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

const (
	orgA = "11111111-1111-1111-1111-111111111111"
	orgB = "22222222-2222-2222-2222-222222222222"
)

var bothReceivable = fakeEntitlements{granted: map[string]bool{"receivable.enabled": true, "receivable.online_payment": true}}

func tenantCard(id string, feats ...domain.ProductFeature) domain.Product {
	return pub(id, "Layanan", 0, "code-"+id, 0, pricing.Monthly, "100000.00", feats...)
}

func TestTenantListingsOnlyOwnProducts(t *testing.T) {
	tcrm := boolFeat("tcrm", true)
	repo := orgPublicRepo{byOrg: map[string][]domain.Product{
		orgA: {tenantCard("a1", tcrm)},
		orgB: {tenantCard("b1", tcrm)},
	}}
	svc := NewPublicListingService(repo, newFakeRegistry(), fakeScopes{}).WithEntitlements(bothReceivable)
	got, err := svc.TenantListings(context.Background(), tenantScope(t, orgA))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Categories) != 1 || len(got.Categories[0].Listings) != 1 || got.Categories[0].Listings[0].Variants[0].ProductID != "a1" {
		t.Fatalf("got = %+v", got)
	}
}

// ctx tanpa tenant context: gerbang needs_update tidak boleh bergantung pada ctx.
func TestTenantListingsExcludesNeedsUpdate(t *testing.T) {
	repo := orgPublicRepo{byOrg: map[string][]domain.Product{orgA: {
		tenantCard("ok", boolFeat("tcrm", true)),
		tenantCard("stale", boolFeat("crm", true)), // crm bukan tenant_offerable
		tenantCard("gone", boolFeat("hilang", true)),
	}}}
	svc := NewPublicListingService(repo, newFakeRegistry(), fakeScopes{}).WithEntitlements(bothReceivable)
	got, err := svc.TenantListings(context.Background(), tenantScope(t, orgA))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Categories) != 1 || len(got.Categories[0].Listings) != 1 || got.Categories[0].Listings[0].Variants[0].ProductID != "ok" {
		t.Fatalf("got = %+v", got)
	}
}

func TestTenantListingsCheckoutGate(t *testing.T) {
	repo := orgPublicRepo{byOrg: map[string][]domain.Product{orgA: {tenantCard("a1", boolFeat("tcrm", true))}}}
	cases := []struct {
		name string
		ent  EntitlementChecker
		want bool
	}{
		{"keduanya", bothReceivable, true},
		{"tanpa online_payment", fakeEntitlements{granted: map[string]bool{"receivable.enabled": true}}, false},
		{"tanpa enabled", fakeEntitlements{granted: map[string]bool{"receivable.online_payment": true}}, false},
		{"tanpa checker", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewPublicListingService(repo, newFakeRegistry(), fakeScopes{})
			if tc.ent != nil {
				svc = svc.WithEntitlements(tc.ent)
			}
			got, err := svc.TenantListings(context.Background(), tenantScope(t, orgA))
			if err != nil {
				t.Fatal(err)
			}
			if v := got.Categories[0].Listings[0].Variants[0]; v.CheckoutEnabled != tc.want {
				t.Fatalf("checkout_enabled = %v, want %v", v.CheckoutEnabled, tc.want)
			}
		})
	}
}

// Produk tak layak checkout tetap false walau entitlement lengkap.
func TestTenantListingsCheckoutNeedsValidProduct(t *testing.T) {
	free := tenantCard("free", boolFeat("tcrm", true))
	free.BasePrice = "0.00"
	repo := orgPublicRepo{byOrg: map[string][]domain.Product{orgA: {free}}}
	svc := NewPublicListingService(repo, newFakeRegistry(), fakeScopes{}).WithEntitlements(bothReceivable)
	got, err := svc.TenantListings(context.Background(), tenantScope(t, orgA))
	if err != nil {
		t.Fatal(err)
	}
	if got.Categories[0].Listings[0].Variants[0].CheckoutEnabled {
		t.Fatal("harga 0 tidak boleh checkout")
	}
}

// Regresi: listing platform tak terpengaruh entitlement maupun gerbang needs_update.
func TestPlatformListingsUnaffectedByTenantGate(t *testing.T) {
	repo := fakePublicRepo{products: []domain.Product{pub("p", "Zyad Cloud", 0, "pro", 0, pricing.Monthly, "100000.00", boolFeat("crm", true))}}
	svc := NewPublicListingService(repo, newFakeRegistry(), fakeScopes{}).WithEntitlements(fakeEntitlements{})
	got, err := svc.Listings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Categories) != 1 || !got.Categories[0].Listings[0].Variants[0].CheckoutEnabled {
		t.Fatalf("got = %+v", got)
	}
}
