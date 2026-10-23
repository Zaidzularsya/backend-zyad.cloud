//go:build integration

package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"zyad.cloud/internal/platform/database/testutil"
)

// FindForTenant: hanya workspace yang dimiliki pembeli DAN (tertaut ke company tenant ini ATAU
// ditandai dibuat checkout tenant ini). Workspace pribadi / milik tenant lain tidak dipakai ulang.
func TestTenantBuyerWorkspacesFindForTenantIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var userID string
	if err := db.QueryRow(ctx, `INSERT INTO users (name, email, status) VALUES ('Buyer', $1, 'active') RETURNING id::text`,
		"tc-buyer-"+suffix+"@test.local").Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { db.Exec(context.Background(), `DELETE FROM users WHERE id = $1::uuid`, userID) })

	newOrg := func(slug, metadata string) string {
		var id string
		if err := db.QueryRow(ctx, `
			INSERT INTO organizations (type, slug, name, status, data_placement, metadata)
			VALUES ('customer', $1, $1, 'active', 'shared', $2::jsonb) RETURNING id::text`, slug+"-"+suffix, metadata).Scan(&id); err != nil {
			t.Fatalf("insert org %s: %v", slug, err)
		}
		t.Cleanup(func() {
			db.Exec(context.Background(), `DELETE FROM crm_companies WHERE organization_id = $1::uuid OR tenant_organization_id = $1::uuid`, id)
			db.Exec(context.Background(), `DELETE FROM organization_memberships WHERE organization_id = $1::uuid`, id)
			db.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1::uuid`, id)
		})
		return id
	}
	own := func(orgID string) {
		if _, err := db.Exec(ctx, `
			INSERT INTO organization_memberships (organization_id, user_id, status, is_owner) VALUES ($1::uuid, $2::uuid, 'active', true)`,
			orgID, userID); err != nil {
			t.Fatalf("membership: %v", err)
		}
	}

	tenantA := newOrg("tc-tenant-a", `{}`)
	tenantB := newOrg("tc-tenant-b", `{}`)
	personal := newOrg("tc-personal", `{}`)
	own(personal)
	w := tenantBuyerWorkspaces{db: db}

	if _, found, err := w.FindForTenant(ctx, tenantA, userID); err != nil || found {
		t.Fatalf("personal workspace must not be reused: found=%v err=%v", found, err)
	}

	// (b) ditandai dibuat checkout tenant A, belum tertaut.
	pending := newOrg("tc-pending", fmt.Sprintf(`{"%s": "%s"}`, tenantCheckoutMetadataKey, tenantA))
	own(pending)
	if id, found, err := w.FindForTenant(ctx, tenantA, userID); err != nil || !found || id != pending {
		t.Fatalf("pending: id=%s found=%v err=%v", id, found, err)
	}
	if _, found, _ := w.FindForTenant(ctx, tenantB, userID); found {
		t.Fatal("tenant B must not see tenant A's workspace")
	}

	// (a) tertaut ke company milik tenant B.
	linked := newOrg("tc-linked", `{}`)
	own(linked)
	if _, err := db.Exec(ctx, `INSERT INTO crm_companies (organization_id, name, tenant_organization_id) VALUES ($1::uuid, 'Buyer', $2::uuid)`,
		tenantB, linked); err != nil {
		t.Fatalf("company: %v", err)
	}
	if id, found, err := w.FindForTenant(ctx, tenantB, userID); err != nil || !found || id != linked {
		t.Fatalf("linked: id=%s found=%v err=%v", id, found, err)
	}
}
