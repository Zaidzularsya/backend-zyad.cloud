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
