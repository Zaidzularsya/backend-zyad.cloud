package app

import (
	"context"
	"fmt"
	"time"

	"zyad.cloud/internal/config"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	"zyad.cloud/internal/platform/database"
)

// R4-S5: workspace lama (yang aksesnya bersumber dari plan) dipindah ke produk default/contract
// sebelum tabel plan/subscription/billing lama dihapus (migration 000154).

const maxPreflightDetails = 50

const planMigrationReason = "R4-S5 migrated to default product"

// DefaultAccessProvisioner memberi workspace paket gratis (lihat defaultAccessProvisioner).
type DefaultAccessProvisioner interface {
	ProvisionDefaultAccess(ctx context.Context, organizationID string) error
}

type PreflightReport struct {
	PaidActiveSubscriptions   int      // customer_subscriptions plan type paid|enterprise, status active|trialing|past_due|grace_period
	OpenLegacyInvoices        int      // billing_invoices status draft|open
	WorkspacesNeedingBackfill int      // org customer tanpa entitlement aktif contract/default
	Details                   []string // org slug + alasan, maks 50 baris
}

// Clean: aman menjalankan migration 000154 (backfill boleh belum jalan; guard SQL memeriksanya sendiri).
func (r PreflightReport) Clean() bool {
	return r.PaidActiveSubscriptions == 0 && r.OpenLegacyInvoices == 0
}

func (r *PreflightReport) addDetail(line string) {
	if len(r.Details) < maxPreflightDetails {
		r.Details = append(r.Details, line)
	}
}

func RunPreflight(ctx context.Context, db *database.Pool) (PreflightReport, error) {
	var report PreflightReport

	hasSubs, err := tableExists(ctx, db, "customer_subscriptions", "product_plans")
	if err != nil {
		return report, err
	}
	if hasSubs {
		rows, err := db.Query(ctx, `
			SELECT o.slug, p.plan_type, s.status
			FROM customer_subscriptions s
			JOIN product_plans p ON p.id = s.plan_id
			JOIN organizations o ON o.id = s.organization_id
			WHERE p.plan_type IN ('paid','enterprise')
				AND s.status IN ('active','trialing','past_due','grace_period')
			ORDER BY o.slug`)
		if err != nil {
			return report, fmt.Errorf("preflight subscriptions: %w", err)
		}
		for rows.Next() {
			var slug, planType, status string
			if err := rows.Scan(&slug, &planType, &status); err != nil {
				rows.Close()
				return report, err
			}
			report.PaidActiveSubscriptions++
			report.addDetail(fmt.Sprintf("%s: subscription %s %s", slug, planType, status))
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return report, err
		}
	}

	hasInvoices, err := tableExists(ctx, db, "billing_invoices")
	if err != nil {
		return report, err
	}
	if hasInvoices {
		rows, err := db.Query(ctx, `
			SELECT o.slug, i.status
			FROM billing_invoices i
			JOIN organizations o ON o.id = i.organization_id
			WHERE i.status IN ('draft','open')
			ORDER BY o.slug`)
		if err != nil {
			return report, fmt.Errorf("preflight invoices: %w", err)
		}
		for rows.Next() {
			var slug, status string
			if err := rows.Scan(&slug, &status); err != nil {
				rows.Close()
				return report, err
			}
			report.OpenLegacyInvoices++
			report.addDetail(fmt.Sprintf("%s: invoice billing %s", slug, status))
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return report, err
		}
	}

	rows, err := db.Query(ctx, `
		SELECT o.slug
		FROM organizations o
		WHERE o.type = 'customer' AND o.deleted_at IS NULL
			AND NOT EXISTS (
				SELECT 1 FROM organization_entitlements e
				WHERE e.organization_id = o.id
					AND e.source IN ('contract','default')
					AND e.status = 'active'
					AND e.effective_from <= now()
					AND (e.effective_until IS NULL OR e.effective_until > now()))
		ORDER BY o.slug`)
	if err != nil {
		return report, fmt.Errorf("preflight backfill candidates: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return report, err
		}
		report.WorkspacesNeedingBackfill++
		report.addDetail(slug + ": belum punya akses contract/default")
	}
	return report, rows.Err()
}

// tableExists benar bila semua tabel ada; preflight harus tetap jalan setelah migration 000154.
func tableExists(ctx context.Context, db *database.Pool, tables ...string) (bool, error) {
	for _, table := range tables {
		var exists bool
		if err := db.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&exists); err != nil {
			return false, err
		}
		if !exists {
			return false, nil
		}
	}
	return true, nil
}

type BackfillResult struct{ Granted, SkippedContract, ExpiredPlanRows int }

// accessEntitlements adalah bagian EntitlementRepository yang dipakai backfill.
type accessEntitlements interface {
	HasActiveSource(ctx context.Context, organizationID string, source organizationmodel.EntitlementSource, at time.Time) (bool, error)
	ExpireBySource(ctx context.Context, organizationID string, source organizationmodel.EntitlementSource, sourceReference *string,
		effectiveUntil time.Time, actorUserID string, reason string) (int64, error)
}

// RunBackfillDefault memberi tiap workspace customer akses dari sumber baru lalu meng-expire baris
// `plan`. Idempoten: workspace yang sudah punya contract/default tidak diberi grant lagi.
func RunBackfillDefault(
	ctx context.Context,
	db *database.Pool,
	provisioner DefaultAccessProvisioner,
	entitlements *organizationrepo.EntitlementRepository,
	actorUserID string,
) (BackfillResult, error) {
	orgIDs, err := listCustomerOrganizationIDs(ctx, db)
	if err != nil {
		return BackfillResult{}, err
	}
	return backfillDefault(ctx, orgIDs, provisioner, entitlements, actorUserID, time.Now())
}

func listCustomerOrganizationIDs(ctx context.Context, db *database.Pool) ([]string, error) {
	rows, err := db.Query(ctx, `
		SELECT id::text FROM organizations
		WHERE type = 'customer' AND deleted_at IS NULL
		ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("list customer organizations: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func backfillDefault(
	ctx context.Context,
	orgIDs []string,
	provisioner DefaultAccessProvisioner,
	entitlements accessEntitlements,
	actorUserID string,
	now time.Time,
) (BackfillResult, error) {
	var result BackfillResult
	for _, orgID := range orgIDs {
		hasContract, err := entitlements.HasActiveSource(ctx, orgID, organizationmodel.EntitlementSourceContract, now)
		if err != nil {
			return result, fmt.Errorf("org %s: check contract: %w", orgID, err)
		}
		if hasContract {
			result.SkippedContract++
		} else {
			hasDefault, err := entitlements.HasActiveSource(ctx, orgID, organizationmodel.EntitlementSourceDefault, now)
			if err != nil {
				return result, fmt.Errorf("org %s: check default: %w", orgID, err)
			}
			if !hasDefault {
				// Grant dulu baru expire plan: bila terputus di antaranya, workspace tidak pernah tanpa akses.
				if err := provisioner.ProvisionDefaultAccess(ctx, orgID); err != nil {
					return result, fmt.Errorf("org %s: provision default: %w", orgID, err)
				}
				result.Granted++
			}
		}
		expired, err := entitlements.ExpireBySource(ctx, orgID, organizationmodel.EntitlementSourcePlan, nil, now, actorUserID, planMigrationReason)
		if err != nil {
			return result, fmt.Errorf("org %s: expire plan rows: %w", orgID, err)
		}
		result.ExpiredPlanRows += int(expired)
	}
	return result, nil
}

// RunBackfillDefaultFromConfig merakit provisioner paket gratis dan aktor bot dari konfigurasi.
func RunBackfillDefaultFromConfig(ctx context.Context, cfg config.Config, db *database.Pool) (BackfillResult, error) {
	botID, err := (&botUser{db: db, email: cfg.SelfServe.BotEmail}).ID(ctx)
	if err != nil {
		return BackfillResult{}, err
	}
	return RunBackfillDefault(ctx, db, workerDefaultAccess(cfg, db), organizationrepo.NewEntitlementRepository(db), botID)
}
