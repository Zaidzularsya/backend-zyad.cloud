//go:build integration

package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"zyad.cloud/internal/modules/catalog/repository"
	"zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/database/testutil"
	"zyad.cloud/internal/shared/pricing"
)

func setupOrgs(t *testing.T, db *database.Pool, tenants testutil.TenantPair) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status) VALUES
		($1, 'customer', 'organization-a', 'Organization A', 'active'),
		($2, 'customer', 'organization-b', 'Organization B', 'active')
		ON CONFLICT DO NOTHING`, tenants.A.OrganizationID, tenants.B.OrganizationID); err != nil {
		t.Fatalf("insert organizations: %v", err)
	}
	t.Cleanup(func() {
		for _, orgID := range []string{tenants.A.OrganizationID, tenants.B.OrganizationID} {
			tx, err := db.Begin(ctx)
			if err != nil {
				return
			}
			_, _ = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", orgID)
			_, _ = tx.Exec(ctx, "DELETE FROM catalog_products WHERE organization_id = $1", orgID)
			_, _ = tx.Exec(ctx, "DELETE FROM catalog_product_categories WHERE organization_id = $1", orgID)
			_ = tx.Commit(ctx)
		}
	})
}

func TestProductCRUDAndSKURules(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupOrgs(t, db, tenants)
	products := repository.NewProductRepository(db)
	categories := repository.NewCategoryRepository(db)

	cat, err := categories.Create(ctx, tenants.A.Scope, "Internet", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := products.Create(ctx, tenants.A.Scope, repository.CreateProductParams{
		CategoryID: cat.ID, SKU: "NET-50", Name: "Internet 50 Mbps", Unit: "bulan",
		BasePrice: "350000", TaxPercent: "11", IsActive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.CategoryName != "Internet" || p.BasePrice != "350000.00" || p.TaxPercent != "11.00" || p.Currency != "IDR" {
		t.Fatalf("product = %+v", p)
	}

	if p.Pricing != (pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Prepaid}) {
		t.Fatalf("default pricing = %+v", p.Pricing)
	}
	rec := pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Postpaid}
	rp, err := products.Create(ctx, tenants.A.Scope, repository.CreateProductParams{SKU: "NET-REC", Name: "Internet langganan", Unit: "bulan", BasePrice: "300000", TaxPercent: "0", Pricing: rec, IsActive: true})
	if err != nil {
		t.Fatal(err)
	}
	if rp.Pricing != rec {
		t.Fatalf("recurring pricing round-trip = %+v", rp.Pricing)
	}
	oneTime := pricing.Attributes{ChargeType: pricing.OneTime, PaymentTiming: pricing.Prepaid}
	if up, err := products.Update(ctx, tenants.A.Scope, rp.ID, repository.UpdateProductParams{Pricing: &oneTime}); err != nil || up.Pricing != oneTime {
		t.Fatalf("update to one_time = %+v err=%v", up.Pricing, err)
	}

	// SKU unik case-insensitive.
	if _, err := products.Create(ctx, tenants.A.Scope, repository.CreateProductParams{SKU: "net-50", Name: "Dup", Unit: "pcs", BasePrice: "1", TaxPercent: "0", IsActive: true}); !errors.Is(err, repository.ErrSKUExists) {
		t.Fatalf("dup sku err = %v", err)
	}
	// Dua produk tanpa SKU boleh.
	for i := 0; i < 2; i++ {
		if _, err := products.Create(ctx, tenants.A.Scope, repository.CreateProductParams{Name: "Jasa instalasi", Unit: "paket", BasePrice: "500000", TaxPercent: "0", IsActive: true}); err != nil {
			t.Fatalf("no sku #%d: %v", i, err)
		}
	}
	// Tenant B tidak melihat produk A.
	if _, err := products.FindByID(ctx, tenants.B.Scope, p.ID); err == nil {
		t.Fatal("tenant B must not read tenant A product")
	}

	// Nonaktifkan → tidak muncul di filter aktif.
	inactive := false
	if _, err := products.Update(ctx, tenants.A.Scope, p.ID, repository.UpdateProductParams{IsActive: &inactive}); err != nil {
		t.Fatal(err)
	}
	active := true
	list, total, err := products.List(ctx, tenants.A.Scope, repository.ProductListFilter{IsActive: &active, Limit: 50})
	if err != nil || total != 3 || len(list) != 3 {
		t.Fatalf("active list total=%d len=%d err=%v", total, len(list), err)
	}
	found, _ := products.FindByIDs(ctx, tenants.A.Scope, []string{p.ID})
	if _, ok := found[p.ID]; !ok {
		t.Fatal("FindByIDs must return inactive products")
	}

	// Hapus kategori → produk tetap, kategori kosong.
	if err := categories.Delete(ctx, tenants.A.Scope, cat.ID, ""); err != nil {
		t.Fatal(err)
	}
	again, err := products.FindByID(ctx, tenants.A.Scope, p.ID)
	if err != nil || again.CategoryID != nil {
		t.Fatalf("after category delete: %+v err=%v", again, err)
	}

	// Hapus produk → hilang dari FindByIDs dan SKU bebas dipakai lagi.
	if err := products.Delete(ctx, tenants.A.Scope, p.ID, ""); err != nil {
		t.Fatal(err)
	}
	if found, _ := products.FindByIDs(ctx, tenants.A.Scope, []string{p.ID}); len(found) != 0 {
		t.Fatal("deleted product must not be returned")
	}
	if _, err := products.Create(ctx, tenants.A.Scope, repository.CreateProductParams{SKU: "NET-50", Name: "Baru", Unit: "bulan", BasePrice: "1", TaxPercent: "0", IsActive: true}); err != nil {
		t.Fatalf("reuse sku after delete: %v", err)
	}
}

func TestCategoryNameUnique(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupOrgs(t, db, tenants)
	categories := repository.NewCategoryRepository(db)
	if _, err := categories.Create(ctx, tenants.A.Scope, "Hardware", 0, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := categories.Create(ctx, tenants.A.Scope, "hardware", 1, ""); !errors.Is(err, repository.ErrCategoryNameExists) {
		t.Fatalf("dup category err = %v", err)
	}
	if _, err := categories.Create(ctx, tenants.B.Scope, "Hardware", 0, ""); err != nil {
		t.Fatalf("other tenant same name: %v", err)
	}
}

func TestCatalogListingConstraints(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupOrgs(t, db, tenants)
	categories := repository.NewCategoryRepository(db)
	catA, err := categories.Create(ctx, tenants.A.Scope, "Zyad Cloud", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	catB, err := categories.Create(ctx, tenants.B.Scope, "Zyad Cloud", 0, "")
	if err != nil {
		t.Fatal(err)
	}

	insert := func(org string, category any, code any, public bool, freq string) error {
		tx, err := db.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", org); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO catalog_products (organization_id, category_id, name, unit, charge_type, billing_frequency, payment_timing, is_public, listing_code)
			VALUES ($1, $2, 'P', 'bulan', 'recurring', $3, 'prepaid', $4, $5)`, org, category, freq, public, code)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	orgA, orgB := tenants.A.OrganizationID, tenants.B.OrganizationID
	if err := insert(orgA, catA.ID, nil, true, "monthly"); err == nil {
		t.Error("public without listing_code must fail")
	}
	if err := insert(orgA, nil, "freelancer", true, "monthly"); err == nil {
		t.Error("public without category must fail")
	}
	if err := insert(orgA, catA.ID, "Freelancer", true, "monthly"); err == nil {
		t.Error("uppercase listing_code must fail")
	}
	if err := insert(orgA, catA.ID, "freelancer", true, "monthly"); err != nil {
		t.Fatalf("first public product: %v", err)
	}
	if err := insert(orgA, catA.ID, "freelancer", true, "monthly"); err == nil {
		t.Error("duplicate listing_code+frequency must fail")
	}
	if err := insert(orgA, catA.ID, "freelancer", true, "annual"); err != nil {
		t.Errorf("other frequency must pass: %v", err)
	}
	if err := insert(orgB, catB.ID, "freelancer", true, "monthly"); err != nil {
		t.Errorf("other org same listing must pass: %v", err)
	}
}

func TestProductFeaturesAndListing(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupOrgs(t, db, tenants)
	products := repository.NewProductRepository(db)
	categories := repository.NewCategoryRepository(db)
	scope := tenants.A.Scope

	cat, err := categories.Create(ctx, scope, "Zyad Cloud", 3, "")
	if err != nil {
		t.Fatal(err)
	}
	monthly := pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Monthly, PaymentTiming: pricing.Prepaid}
	feats := []repository.FeatureValue{
		{FeatureKey: "users", Value: json.RawMessage(`5`), DisplayLabel: "Hingga 5 user", Position: 1},
		{FeatureKey: "crm", Value: json.RawMessage(`true`), Position: 0},
	}
	p, err := products.Create(ctx, scope, repository.CreateProductParams{
		CategoryID: cat.ID, SKU: "FL-M", Name: "Freelancer", Unit: "bulan", BasePrice: "150000", TaxPercent: "11",
		Pricing: monthly, IsActive: true,
		Listing:  repository.ListingParams{IsPublic: true, ListingCode: "freelancer", ListingOrder: 2},
		Features: feats,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !p.IsPublic || p.ListingCode != "freelancer" || p.ListingOrder != 2 || len(p.Features) != 2 ||
		p.Features[0].FeatureKey != "crm" || p.Features[1].FeatureKey != "users" || p.Features[1].DisplayLabel != "Hingga 5 user" {
		t.Fatalf("created = %+v", p)
	}

	one := []repository.FeatureValue{{FeatureKey: "crm", Value: json.RawMessage(`true`)}}
	up, err := products.Update(ctx, scope, p.ID, repository.UpdateProductParams{Features: &one})
	if err != nil || len(up.Features) != 1 {
		t.Fatalf("update features = %+v err=%v", up.Features, err)
	}
	name := "Freelancer 2"
	up, err = products.Update(ctx, scope, p.ID, repository.UpdateProductParams{Name: &name})
	if err != nil || len(up.Features) != 1 || !up.IsPublic {
		t.Fatalf("nil features/listing must stay: %+v err=%v", up, err)
	}
	empty := []repository.FeatureValue{}
	up, err = products.Update(ctx, scope, p.ID, repository.UpdateProductParams{Features: &empty})
	if err != nil || len(up.Features) != 0 {
		t.Fatalf("empty features = %+v err=%v", up.Features, err)
	}
	if _, err := products.Update(ctx, scope, p.ID, repository.UpdateProductParams{Features: &feats}); err != nil {
		t.Fatal(err)
	}

	// Listing kembar → ErrListingExists, bukan error mentah.
	_, err = products.Create(ctx, scope, repository.CreateProductParams{
		CategoryID: cat.ID, SKU: "FL-M2", Name: "Dup", Unit: "bulan", BasePrice: "1", TaxPercent: "0",
		Pricing: monthly, IsActive: true, Listing: repository.ListingParams{IsPublic: true, ListingCode: "freelancer"},
	})
	if !errors.Is(err, repository.ErrListingExists) {
		t.Fatalf("dup listing err = %v", err)
	}
	annual := pricing.Attributes{ChargeType: pricing.Recurring, Frequency: pricing.Annual, PaymentTiming: pricing.Prepaid}
	py, err := products.Create(ctx, scope, repository.CreateProductParams{
		CategoryID: cat.ID, SKU: "FL-Y", Name: "Freelancer Y", Unit: "tahun", BasePrice: "1500000", TaxPercent: "11",
		Pricing: annual, IsActive: true, Listing: repository.ListingParams{IsPublic: true, ListingCode: "freelancer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Update ke listing yang bentrok juga dipetakan.
	if _, err := products.Update(ctx, scope, py.ID, repository.UpdateProductParams{Pricing: &monthly}); !errors.Is(err, repository.ErrListingExists) {
		t.Fatalf("update dup listing err = %v", err)
	}
	// Produk privat dan nonaktif tidak ikut ListPublic.
	if _, err := products.Create(ctx, scope, repository.CreateProductParams{SKU: "PRIV", Name: "Privat", Unit: "pcs", BasePrice: "1", TaxPercent: "0", IsActive: true}); err != nil {
		t.Fatal(err)
	}
	pub, err := products.ListPublic(ctx, scope)
	if err != nil || len(pub) != 2 {
		t.Fatalf("ListPublic len=%d err=%v", len(pub), err)
	}
	for _, x := range pub {
		if x.CategoryName != "Zyad Cloud" || x.CategoryPosition != 3 {
			t.Fatalf("category join = %+v", x)
		}
	}
	var withFeatures int
	for _, x := range pub {
		if x.ID == p.ID {
			withFeatures = len(x.Features)
		}
	}
	if withFeatures != 2 {
		t.Fatalf("ListPublic features = %d", withFeatures)
	}
	inactive := false
	if _, err := products.Update(ctx, scope, py.ID, repository.UpdateProductParams{IsActive: &inactive}); err != nil {
		t.Fatal(err)
	}
	if pub, _ := products.ListPublic(ctx, scope); len(pub) != 1 {
		t.Fatalf("inactive must be hidden, len=%d", len(pub))
	}

	got, err := products.FindByIDs(ctx, scope, []string{p.ID})
	if err != nil || len(got[p.ID].Features) != 2 {
		t.Fatalf("FindByIDs features = %+v err=%v", got[p.ID].Features, err)
	}
	// Isolasi tenant.
	if other, _ := products.ListPublic(ctx, tenants.B.Scope); len(other) != 0 {
		t.Fatal("tenant B must not see tenant A listings")
	}
	if other, _ := products.FindByIDs(ctx, tenants.B.Scope, []string{p.ID}); len(other) != 0 {
		t.Fatal("tenant B must not read tenant A product")
	}
}
