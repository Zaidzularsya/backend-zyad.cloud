package app

import (
	"context"
	"fmt"
	"time"

	crmdomain "zyad.cloud/internal/modules/crm/domain"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	productrepo "zyad.cloud/internal/modules/product/repository"
	receivabledomain "zyad.cloud/internal/modules/receivable/domain"
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

// registerDomainQuotaMeters registers the total-domain meter: all domains of
// the organization that are not soft-deleted, excluding the global 'platform'
// type (not tenant-created), so Free's cap of 1 means one platform subdomain.
func registerDomainQuotaMeters(reg *organizationservice.QuotaMeterRegistry, db *database.Pool) {
	reg.Register(organizationservice.FeatureDomainMaxDomains, scopedCountMeter{db: db, query: `SELECT COUNT(*) FROM organization_domains
		WHERE organization_id = $1 AND deleted_at IS NULL AND type IN ('subdomain', 'custom')`})
}

// registerMonthlyQuotaMeters registers the meters of the *PerMonth features.
// They count rows created since periodStart (the first day of the current
// month in business time, see organizationservice.PeriodStart), so unlike the
// standing caps above the count resets by itself every month.
//
//   - quotations: deleted_at IS NULL AND revision_of_id IS NULL. A revision
//     (Revise) supersedes its predecessor, so it is NOT a new quotation: revising
//     is never penalised and a quotation counts once, in the month it was first
//     created. Soft-deleted quotations free their room, like deleted leads.
//   - sales orders: status <> 'cancelled' (the table has no deleted_at). Orders are
//     created automatically from approved quotations; a cancelled one frees room.
//   - invoices: status <> 'void' (the table has no deleted_at). Drafts count (they
//     are real documents the tenant created), voided ones free room. Invoices from
//     the recurring billing run ARE counted here even though they are never blocked.
func registerMonthlyQuotaMeters(reg *organizationservice.QuotaMeterRegistry, db *database.Pool) {
	reg.Register(crmdomain.FeatureCRMMaxQuotationsPerMonth, scopedMonthlyCountMeter{db: db, query: `SELECT COUNT(*) FROM crm_quotations
		WHERE organization_id = $1 AND deleted_at IS NULL AND revision_of_id IS NULL AND created_at >= $2`})
	reg.Register(crmdomain.FeatureCRMMaxSalesOrdersPerMonth, scopedMonthlyCountMeter{db: db, query: `SELECT COUNT(*) FROM crm_sales_orders
		WHERE organization_id = $1 AND status <> 'cancelled' AND created_at >= $2`})
	reg.Register(receivabledomain.FeatureReceivableMaxInvoicesPerMonth, scopedMonthlyCountMeter{db: db, query: `SELECT COUNT(*) FROM receivable_invoices
		WHERE organization_id = $1 AND status <> 'void' AND created_at >= $2`})
}

// newQuotaMeterRegistry registers every quota meter. The enforcer and the
// entitlement summary both build from it so they always agree on usage.
func newQuotaMeterRegistry(db *database.Pool) *organizationservice.QuotaMeterRegistry {
	meters := organizationservice.NewQuotaMeterRegistry()
	registerCRMQuotaMeters(meters, db)
	registerMonthlyQuotaMeters(meters, db)
	registerDomainQuotaMeters(meters, db)
	return meters
}

// newCRMQuotaEnforcer builds the QuotaEnforcer used by every CRM creation path
// (API services and the worker's WhatsApp auto-lead), with the CRM meters registered.
// It also carries the monthly quotation / sales order / invoice meters, so the same
// enforcer can be handed to the receivable module.
func newCRMQuotaEnforcer(guard *organizationservice.EntitlementGuard, db *database.Pool) *organizationservice.QuotaEnforcer {
	return organizationservice.NewQuotaEnforcer(
		guard,
		catalogFeatureRegistry{features: productrepo.NewFeatureRepository(db)},
		newQuotaMeterRegistry(db),
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

// scopedMonthlyCountMeter is scopedCountMeter for monthly features: the query
// takes $1 = organization id and $2 = period start. created_at columns are
// "timestamp without time zone" holding UTC (the session is pinned to UTC), while
// periodStart is midnight in business time, so it is converted to UTC first;
// otherwise the driver would drop the zone and shift the boundary by the offset.
type scopedMonthlyCountMeter struct {
	db    *database.Pool
	query string
}

func (m scopedMonthlyCountMeter) Used(ctx context.Context, organizationID string, periodStart *time.Time) (int64, error) {
	if periodStart == nil {
		return 0, fmt.Errorf("monthly quota meter requires a period start")
	}
	tx, err := m.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", organizationID); err != nil {
		return 0, err
	}
	var total int64
	if err := tx.QueryRow(ctx, m.query, organizationID, periodStart.UTC()).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}
