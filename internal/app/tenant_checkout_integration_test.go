//go:build integration

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	crmdomain "zyad.cloud/internal/modules/crm/domain"
	crmrepo "zyad.cloud/internal/modules/crm/repository"
	crmservice "zyad.cloud/internal/modules/crm/service"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	productrepo "zyad.cloud/internal/modules/product/repository"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
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

// Fakes sempit untuk TenantAccess: hanya contract/SO/company yang di-stub; writer entitlement + DB nyata.
type tcOrders struct {
	crmrepo.SalesOrderRepository
	so crmdomain.SalesOrder
}

func (o tcOrders) FindByID(context.Context, coretenant.Scope, string) (crmdomain.SalesOrder, error) {
	return o.so, nil
}

type tcCompanies struct {
	crmrepo.CompanyRepository
	company crmdomain.Company
}

func (c tcCompanies) FindByID(context.Context, coretenant.Scope, string) (crmdomain.Company, error) {
	return c.company, nil
}

type tcContracts struct {
	info crmservice.ContractAccessInfo
	paid *bool
}

func (c tcContracts) ContractAccess(context.Context, coretenant.Scope, string) (crmservice.ContractAccessInfo, error) {
	return c.info, nil
}
func (c tcContracts) ContractHasPaidInvoice(context.Context, coretenant.Scope, string) (bool, error) {
	return *c.paid, nil
}

type tcInvoices struct{}

func (tcInvoices) InvoiceStatus(context.Context, coretenant.Scope, string) (string, string, error) {
	return "issued", "INV-1", nil
}

// Urutan end-to-end: checkout tenant → invoice belum lunas → workspace pembeli BELUM punya
// organization_entitlements → invoice lunas → baris source='contract' untuk fitur snapshot produk.
// Skeleton: butuh DB integrasi dengan fitur `user.max` ada di registry; tidak dijalankan di CI default.
func TestTenantCheckoutPrepaidEntitlementAfterPaidIntegration(t *testing.T) {
	db := testutil.OpenTestDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	var buyerOrg, tenantOrg string
	for slug, dest := range map[string]*string{"tc-buyer-ws-": &buyerOrg, "tc-tenant-": &tenantOrg} {
		if err := db.QueryRow(ctx, `
			INSERT INTO organizations (type, slug, name, status, data_placement, metadata)
			VALUES ('customer', $1, $1, 'active', 'shared', '{}'::jsonb) RETURNING id::text`, slug+suffix).Scan(dest); err != nil {
			t.Fatalf("insert org %s: %v", slug, err)
		}
	}
	t.Cleanup(func() {
		for _, id := range []string{buyerOrg, tenantOrg} {
			db.Exec(context.Background(), `DELETE FROM organization_entitlements WHERE organization_id = $1::uuid`, id)
			db.Exec(context.Background(), `DELETE FROM organizations WHERE id = $1::uuid`, id)
		}
	})

	paid := false
	companyID := "co-1"
	contracts := tcContracts{paid: &paid, info: crmservice.ContractAccessInfo{
		ID: "00000000-0000-0000-0000-0000000000c1", Number: "CTR-TC-1", Status: "active",
		SourceType: string(receivabledomain.SourceSalesOrder), SourceID: "so-1",
		Items: []crmservice.ContractAccessItem{{Prepaid: true, Features: []crmdomain.FeatureSnapshot{
			{FeatureKey: "user.max", Value: json.RawMessage(`5`)},
		}}},
	}}
	access := crmservice.NewTenantAccess(
		tcOrders{so: crmdomain.SalesOrder{ID: "so-1", CompanyID: &companyID, InitialInvoiceID: "inv-1"}},
		tcCompanies{company: crmdomain.Company{TenantOrganizationID: &buyerOrg}},
		contracts, tcInvoices{},
		tenantEntitlementWriter{
			svc:      organizationservice.NewContractEntitlementService(organizationrepo.NewEntitlementRepository(db)),
			registry: catalogFeatureRegistry{features: productrepo.NewFeatureRepository(db)},
		},
	)
	scope := coretenant.Scope{} // fake reader mengabaikan scope

	count := func() int {
		var n int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM organization_entitlements WHERE organization_id = $1::uuid AND source = 'contract'`, buyerOrg).Scan(&n); err != nil {
			t.Fatalf("count entitlements: %v", err)
		}
		return n
	}

	if err := access.SyncContract(ctx, scope, contracts.info.ID, crmservice.SyncContractCreated); err != nil {
		t.Fatalf("sync created: %v", err)
	}
	if n := count(); n != 0 {
		t.Fatalf("unpaid prepaid contract produced %d contract entitlements", n)
	}

	paid = true
	if err := access.SyncContract(ctx, scope, contracts.info.ID, crmservice.SyncInvoicePaid); err != nil {
		t.Fatalf("sync paid: %v", err)
	}
	if n := count(); n == 0 {
		t.Fatal("expected source='contract' entitlement rows after invoice paid")
	}
}
