//go:build integration

package repository_test

import (
	"context"
	"testing"

	"zyad.cloud/internal/modules/crm/repository"
	"zyad.cloud/internal/platform/database/testutil"
)

func TestQuotationItemSnapshotColumnsRoundTrip(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx := context.Background()
	tenants := testutil.NewTenantPair(t)
	setupCRMOrganizations(t, db, tenants)

	q, err := repository.NewQuotationRepository(db).Create(ctx, tenants.A.Scope, repository.CreateQuotationParams{
		QuotationNumber: "QUO-T-0001", Subtotal: "100.00", DiscountTotal: "0.00", TaxTotal: "11.00", GrandTotal: "111.00",
		Items: []repository.QuotationItemInput{{
			Description: "Internet 50 Mbps", Quantity: "1.00", UnitPrice: "100.00", LineTotal: "100.00",
			SKU: "NET-50", Unit: "bulan", TaxPercent: "11", TaxAmount: "11.00",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	it := q.Items[0]
	if it.SKU != "NET-50" || it.Unit != "bulan" || it.TaxPercent != "11.00" || it.TaxAmount != "11.00" || it.ProductID != nil {
		t.Fatalf("item = %+v", it)
	}
	if q.RevisionNo != 0 || q.RevisionOfID != nil || q.PDFAssetID != nil {
		t.Fatalf("quotation = %+v", q)
	}
}
