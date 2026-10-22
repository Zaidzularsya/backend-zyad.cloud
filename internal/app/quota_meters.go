package app

import (
	"context"
	"fmt"
	"time"

	crmdomain "zyad.cloud/internal/modules/crm/domain"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	productrepo "zyad.cloud/internal/modules/product/repository"
	"zyad.cloud/internal/platform/database"
)

// registerCRMQuotaMeters registers the "total" CRM quota meters. All of them
// count rows that currently exist and are "active", ignoring periodStart (the
// features use reset_strategy=never, i.e. a standing cap, not a monthly one).
//
// Definition of "active" (a deliberate product decision, see the entitlement
// upgrade UX spec): a record stops consuming quota when it is soft-deleted or
// has left the working set, so tenants can free room without a plan change.
//
//   - leads:     deleted_at IS NULL and status not in (unqualified, converted).
//     Disqualified and converted leads are terminal history; the converted
//     lead's contact/company/deal are counted on their own meters.
//   - companies: deleted_at IS NULL.
//   - deals:     deleted_at IS NULL and status = 'open'; won/lost deals are closed.
//   - pipelines: deleted_at IS NULL and archived_at IS NULL, matching
//     PipelineRepository.CountActive (the "first pipeline is free" rule).
//
// Contacts (crm.max_contacts) are enforced by ContactService and not metered here.
func registerCRMQuotaMeters(reg *organizationservice.QuotaMeterRegistry, db *database.Pool) {
	reg.Register(crmdomain.FeatureCRMMaxLeads, scopedCountMeter{db: db, query: fmt.Sprintf(
		`SELECT COUNT(*) FROM crm_leads WHERE organization_id = $1 AND deleted_at IS NULL AND status NOT IN ('%s', '%s')`,
		crmdomain.LeadStatusUnqualified, crmdomain.LeadStatusConverted)})
	reg.Register(crmdomain.FeatureCRMMaxCompanies, scopedCountMeter{db: db, query: `SELECT COUNT(*) FROM crm_companies WHERE organization_id = $1 AND deleted_at IS NULL`})
	reg.Register(crmdomain.FeatureCRMMaxDeals, scopedCountMeter{db: db, query: fmt.Sprintf(
		`SELECT COUNT(*) FROM crm_deals WHERE organization_id = $1 AND deleted_at IS NULL AND status = '%s'`,
		crmdomain.DealStatusOpen)})
	reg.Register(crmdomain.FeatureCRMMaxPipelines, scopedCountMeter{db: db, query: `SELECT COUNT(*) FROM crm_pipelines WHERE organization_id = $1 AND deleted_at IS NULL AND archived_at IS NULL`})
}

// newCRMQuotaEnforcer builds the QuotaEnforcer used by every CRM creation path
// (API services and the worker's WhatsApp auto-lead), with the CRM meters registered.
func newCRMQuotaEnforcer(guard *organizationservice.EntitlementGuard, db *database.Pool) *organizationservice.QuotaEnforcer {
	meters := organizationservice.NewQuotaMeterRegistry()
	registerCRMQuotaMeters(meters, db)
	return organizationservice.NewQuotaEnforcer(
		guard,
		catalogFeatureRegistry{features: productrepo.NewFeatureRepository(db)},
		meters,
		time.Now,
	)
}

// scopedCountMeter runs a single-parameter COUNT(*) query ($1 = organization
// id) inside a transaction that sets app.organization_id, the same RLS scoping
// the CRM repositories use (see crm/repository withTx).
type scopedCountMeter struct {
	db    *database.Pool
	query string
}

func (m scopedCountMeter) Used(ctx context.Context, organizationID string, _ *time.Time) (int64, error) {
	tx, err := m.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", organizationID); err != nil {
		return 0, err
	}
	var total int64
	if err := tx.QueryRow(ctx, m.query, organizationID).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}
