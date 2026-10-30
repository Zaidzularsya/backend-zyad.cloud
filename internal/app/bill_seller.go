package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	crmhandler "zyad.cloud/internal/modules/crm/handler"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	"zyad.cloud/internal/platform/database"
)

// billSellerIdentity: identitas layanan untuk membangun scope penjual saat pembeli membaca tagihannya.
const billSellerIdentity = "buyer-bills"

// billSellers menemukan penjual dari workspace pembeli: org pemilik company CRM yang tertaut ke
// workspace itu. Satu workspace tertaut ke paling banyak satu company lintas semua org, jadi
// hasilnya tunggal.
type billSellers struct {
	db       *database.Pool
	resolver *organizationservice.WorkerResolver
}

func newBillSellers(db *database.Pool) billSellers {
	return billSellers{
		db:       db,
		resolver: organizationservice.NewWorkerResolver(organizationrepo.NewOrganizationRepository(db), billSellerIdentity),
	}
}

// SellerOf: crm_companies memakai FORCE RLS dan company ada di org PENJUAL, bukan org pembeli. Query
// lintas-org ini disengaja dan memakai konteks sempit app.crm_workspace_link_lookup (migration 000167):
// hanya baris yang tertaut ke tepat workspace ini yang terbaca, dan app.organization_id dibiarkan
// kosong sehingga baris org lain tetap tertutup. Pembeli datang dari konteks tenant sesi (pemanggil),
// bukan dari input klien.
func (s billSellers) SellerOf(ctx context.Context, buyerOrganizationID string) (crmhandler.BillSeller, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return crmhandler.BillSeller{}, false, err
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(ctx, "SELECT set_config('app.crm_workspace_link_lookup', $1, true)", buyerOrganizationID); err != nil {
		return crmhandler.BillSeller{}, false, err
	}
	var sellerID, sellerName string
	err = tx.QueryRow(ctx, `
		SELECT o.id::text, o.name
		FROM crm_companies c
		JOIN organizations o ON o.id = c.organization_id AND o.deleted_at IS NULL
		WHERE c.tenant_organization_id = $1::uuid AND c.deleted_at IS NULL
		ORDER BY c.created_at, c.id
		LIMIT 1`, buyerOrganizationID).Scan(&sellerID, &sellerName)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmhandler.BillSeller{}, false, nil
	}
	if err != nil {
		return crmhandler.BillSeller{}, false, err
	}

	tc, err := s.resolver.ResolveWorkerOrganization(ctx, sellerID, billSellerIdentity)
	if err != nil {
		return crmhandler.BillSeller{}, false, fmt.Errorf("resolve seller scope: %w", err)
	}
	scope, err := coretenant.NewScope(tc)
	if err != nil {
		return crmhandler.BillSeller{}, false, err
	}
	return crmhandler.BillSeller{Scope: scope, Name: sellerName}, true, nil
}
