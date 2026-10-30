package service

import (
	"context"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/catalog/repository"
	"zyad.cloud/internal/shared/pricing"
)

// memProducts menyimpan produk hasil Create agar ListPublic membaca yang sama.
type memProducts struct {
	repository.ProductRepository
	byOrg map[string][]domain.Product
}

func (m *memProducts) CategoryInScope(context.Context, coretenant.Scope, string) (bool, error) {
	return true, nil
}

func (m *memProducts) Create(_ context.Context, s coretenant.Scope, in repository.CreateProductParams) (domain.Product, error) {
	cat := in.CategoryID
	p := domain.Product{ID: "p-" + in.SKU, OrganizationID: s.OrganizationID(), SKU: in.SKU, Name: in.Name, CategoryID: &cat, CategoryName: "Internet",
		BasePrice: in.BasePrice, TaxPercent: in.TaxPercent, Currency: "IDR", Pricing: in.Pricing, IsActive: in.IsActive,
		IsPublic: in.Listing.IsPublic, ListingCode: in.Listing.ListingCode, ListingOrder: in.Listing.ListingOrder}
	m.byOrg[s.OrganizationID()] = append(m.byOrg[s.OrganizationID()], p)
	return p, nil
}

func (m *memProducts) ListPublic(_ context.Context, s coretenant.Scope) ([]domain.Product, error) {
	var out []domain.Product
	for _, p := range m.byOrg[s.OrganizationID()] {
		if p.IsPublic && p.IsActive {
			out = append(out, p)
		}
	}
	return out, nil
}

// Produk tenant tanpa fitur (mis. paket WiFi, spec §4.2) yang dipublikasikan pemilik
// muncul di katalog publik tenant dan bisa di-checkout.
func TestTenantPublishedProductAppearsInTenantListings(t *testing.T) {
	repo := &memProducts{byOrg: map[string][]domain.Product{}}
	products := NewProductService(repo, newFakeRegistry())
	sc := tenantScope(t, orgA)
	attrs := pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid}
	in := ProductInput{Name: "WiFi 20 Mbps", SKU: "W20", Unit: "bulan", BasePrice: "150000", CategoryID: "c1", IsActive: true, Pricing: attrs,
		Listing: repository.ListingParams{IsPublic: true, ListingCode: "wifi-20"}}
	if _, err := products.Create(context.Background(), sc, in, "u"); err != nil {
		t.Fatal(err)
	}
	hidden := in
	hidden.SKU, hidden.Name, hidden.Listing = "W5", "WiFi 5 Mbps", repository.ListingParams{}
	if _, err := products.Create(context.Background(), sc, hidden, "u"); err != nil {
		t.Fatal(err)
	}

	got, err := NewPublicListingService(repo, newFakeRegistry(), fakeScopes{}).WithEntitlements(bothReceivable).
		TenantListings(context.Background(), sc)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Categories) != 1 || len(got.Categories[0].Listings) != 1 {
		t.Fatalf("got = %+v", got)
	}
	card := got.Categories[0].Listings[0]
	if card.Code != "wifi-20" || len(card.Variants) != 1 || card.Variants[0].ProductID != "p-W20" || !card.Variants[0].CheckoutEnabled {
		t.Fatalf("card = %+v", card)
	}
	// Org lain tidak melihatnya.
	other, err := NewPublicListingService(repo, newFakeRegistry(), fakeScopes{}).WithEntitlements(bothReceivable).
		TenantListings(context.Background(), tenantScope(t, orgB))
	if err != nil || len(other.Categories) != 0 {
		t.Fatalf("org B = %+v err=%v", other, err)
	}
}
