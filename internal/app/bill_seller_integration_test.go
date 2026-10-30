//go:build integration

package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"zyad.cloud/internal/platform/database/testutil"
)

// SellerOf harus menemukan penjual lintas-org di bawah FORCE RLS hanya untuk workspace yang
// diminta, dan tidak membocorkan penjual/pembeli lain. Test ini hanya bermakna bila koneksi uji
// BUKAN superuser (superuser mem-bypass RLS).
func TestBillSellersSellerOfIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var superuser bool
	if err := db.QueryRow(ctx, `SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&superuser); err != nil {
		t.Fatal(err)
	}
	if superuser {
		t.Skip("koneksi uji superuser/BYPASSRLS: RLS tidak berlaku, test tidak bermakna")
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	newOrg := func(label string) string {
		var id string
		if err := db.QueryRow(ctx, `
			INSERT INTO organizations (type, slug, name, status, data_placement)
			VALUES ('customer', $1, $2, 'active', 'shared') RETURNING id::text`,
			"bs-"+label+"-"+suffix, "Penjual "+label).Scan(&id); err != nil {
			t.Fatalf("insert org %s: %v", label, err)
		}
		return id
	}
	seller, otherSeller := newOrg("seller"), newOrg("other-seller")
	buyer, otherBuyer, unlinked := newOrg("buyer"), newOrg("other-buyer"), newOrg("unlinked")
	t.Cleanup(func() {
		c := context.Background()
		for _, org := range []string{seller, otherSeller} {
			tx, err := db.Begin(c)
			if err != nil {
				continue
			}
			_, _ = tx.Exec(c, "SELECT set_config('app.organization_id', $1, true)", org)
			_, _ = tx.Exec(c, `DELETE FROM crm_companies WHERE organization_id = $1::uuid`, org)
			_ = tx.Commit(c)
		}
		for _, id := range []string{seller, otherSeller, buyer, otherBuyer, unlinked} {
			_, _ = db.Exec(c, `DELETE FROM organizations WHERE id = $1::uuid`, id)
		}
	})

	link := func(sellerOrg, buyerOrg, name string) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(context.Background())
		if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", sellerOrg); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO crm_companies (organization_id, name, tenant_organization_id)
			VALUES ($1::uuid, $2, $3::uuid)`, sellerOrg, name, buyerOrg); err != nil {
			t.Fatalf("link company: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	link(seller, buyer, "Pembeli A")
	link(otherSeller, otherBuyer, "Pembeli B")

	sellers := newBillSellers(db)

	got, found, err := sellers.SellerOf(ctx, buyer)
	if err != nil || !found {
		t.Fatalf("buyer: found=%v err=%v", found, err)
	}
	if got.Name != "Penjual seller" || got.Scope.OrganizationID() != seller {
		t.Fatalf("buyer harus menemukan penjualnya sendiri, got name=%q scope=%q want %q", got.Name, got.Scope.OrganizationID(), seller)
	}

	got, found, err = sellers.SellerOf(ctx, otherBuyer)
	if err != nil || !found || got.Scope.OrganizationID() != otherSeller {
		t.Fatalf("pembeli lain harus menemukan penjual lain: found=%v scope=%q err=%v", found, got.Scope.OrganizationID(), err)
	}

	if _, found, err = sellers.SellerOf(ctx, unlinked); err != nil || found {
		t.Fatalf("workspace tanpa tautan tidak boleh menemukan penjual: found=%v err=%v", found, err)
	}
}
