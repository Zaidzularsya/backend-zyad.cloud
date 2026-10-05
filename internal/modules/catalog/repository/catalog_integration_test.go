//go:build integration

package repository_test

import (
	"context"
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
	if err != nil || total != 2 || len(list) != 2 {
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
