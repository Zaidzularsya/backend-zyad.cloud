//go:build integration

package publiclink_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/platform/database/testutil"
	"zyad.cloud/internal/shared/publiclink"
)

func TestPublicLinkDirectory(t *testing.T) {
	ctx := context.Background()
	db := testutil.OpenTestDatabase(t)
	tenants := testutil.NewTenantPair(t)
	if _, err := db.Exec(ctx, `
		INSERT INTO organizations (id, type, slug, name, status)
		VALUES ($1, 'customer', 'publiclink-org-a', 'PL Org A', 'active'),
		       ($2, 'customer', 'publiclink-org-b', 'PL Org B', 'active')
		ON CONFLICT DO NOTHING
	`, tenants.A.OrganizationID, tenants.B.OrganizationID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM public_links WHERE organization_id IN ($1, $2)`, tenants.A.OrganizationID, tenants.B.OrganizationID)
	})

	repo := publiclink.NewRepository(db)
	docID := "aaaaaaaa-0000-0000-0000-00000000d001"
	token, hash, _ := publiclink.NewToken()
	exp := time.Now().UTC().Add(time.Hour)
	if _, err := repo.Insert(ctx, tenants.A.Scope, publiclink.DocumentQuotation, docID, hash, "enc", exp, ""); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindByHash(ctx, publiclink.HashToken(token))
	if err != nil || got.OrganizationID != tenants.A.OrganizationID || got.DocumentID != docID {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	if _, _, err := repo.FindActive(ctx, tenants.B.Scope, publiclink.DocumentQuotation, docID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("tenant B must not see A's link: %v", err)
	}
	_, hash2, _ := publiclink.NewToken()
	if _, err := repo.Insert(ctx, tenants.A.Scope, publiclink.DocumentQuotation, docID, hash2, "enc2", exp, ""); err == nil {
		t.Fatal("second active link must violate unique index")
	}
}
