package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"zyad.cloud/internal/config"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	catalogrepo "zyad.cloud/internal/modules/catalog/repository"
	catalogservice "zyad.cloud/internal/modules/catalog/service"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	productrepo "zyad.cloud/internal/modules/product/repository"
	"zyad.cloud/internal/platform/database"
)

// dbFreePlanStore adalah adapter baca-saja ke database untuk perencanaan re-provision.
type dbFreePlanStore struct {
	db           *database.Pool
	entitlements *organizationrepo.EntitlementRepository
}

func (s dbFreePlanStore) ListDefaultOrganizations(ctx context.Context, at time.Time) ([]FreePlanOrgRef, error) {
	rows, err := s.db.Query(ctx, `
		SELECT o.id::text, o.slug
		FROM organizations o
		WHERE o.type = 'customer' AND o.deleted_at IS NULL
			AND EXISTS (
				SELECT 1 FROM organization_entitlements e
				WHERE e.organization_id = o.id AND e.source = 'default' AND e.status = 'active'
					AND e.effective_from <= $1 AND (e.effective_until IS NULL OR e.effective_until > $1))
		ORDER BY o.slug`, at)
	if err != nil {
		return nil, fmt.Errorf("list organizations with default access: %w", err)
	}
	defer rows.Close()
	var out []FreePlanOrgRef
	for rows.Next() {
		var ref FreePlanOrgRef
		if err := rows.Scan(&ref.ID, &ref.Slug); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

func (s dbFreePlanStore) HasActiveContract(ctx context.Context, organizationID string, at time.Time) (bool, error) {
	return s.entitlements.HasActiveSource(ctx, organizationID, organizationmodel.EntitlementSourceContract, at)
}

func (s dbFreePlanStore) ActiveDefault(ctx context.Context, organizationID string, at time.Time) ([]organizationmodel.Entitlement, error) {
	return s.entitlements.ListActiveBySource(ctx, organizationID, organizationmodel.EntitlementSourceDefault, nil, at)
}

// RunFreePlan menyelaraskan produk FREE dengan FreePlanFeatures lalu me-re-provision workspace ber-akses
// `default`. apply=false (default CLI) hanya mencetak rencana dan tidak menulis apa pun.
func RunFreePlan(ctx context.Context, cfg config.Config, db *database.Pool, apply bool, out io.Writer) error {
	registry := catalogFeatureRegistry{features: productrepo.NewFeatureRepository(db)}
	products := catalogservice.NewProductService(catalogrepo.NewProductRepository(db), registry)
	scope, err := newPlatformScopeResolver(organizationrepo.NewOrganizationRepository(db)).PlatformScope(ctx)
	if err != nil {
		return fmt.Errorf("resolve platform scope: %w", err)
	}
	product, err := products.FindBySKU(ctx, scope, cfg.SelfServe.FreeProductSKU)
	if err != nil {
		return fmt.Errorf("produk Free %q tidak ditemukan di katalog platform (buat dulu lewat UI katalog): %w",
			cfg.SelfServe.FreeProductSKU, err)
	}

	target := FreePlanFeatures()
	keys := make([]string, len(target))
	for i, f := range target {
		keys[i] = f.FeatureKey
	}
	defs, err := registry.FindByKeys(ctx, keys)
	if err != nil {
		return fmt.Errorf("read feature registry: %w", err)
	}
	now := time.Now().UTC()
	store := dbFreePlanStore{db: db, entitlements: organizationrepo.NewEntitlementRepository(db)}
	plan, err := PlanFreePlanReprovision(ctx, store, defs, target, now)
	if err != nil {
		return err
	}
	WriteFreePlanReport(out, DiffProductFeatures(product.Features, target), plan, apply)
	if !apply {
		return nil
	}
	if len(plan.Warnings) > 0 {
		return errors.New("registry belum lengkap (lihat WARNING); jalankan migration terbaru lalu ulangi")
	}

	botID, err := (&botUser{db: db, email: cfg.SelfServe.BotEmail}).ID(ctx)
	if err != nil {
		return err
	}
	values := make([]catalogrepo.FeatureValue, len(target))
	for i, f := range target {
		values[i] = catalogrepo.FeatureValue{FeatureKey: f.FeatureKey, Value: f.Value, Position: f.Position}
	}
	if _, err := products.Update(ctx, scope, product.ID, catalogrepo.UpdateProductParams{Features: &values, UpdatedBy: botID}, true, true); err != nil {
		return fmt.Errorf("update fitur produk Free: %w", err)
	}
	n, err := ApplyFreePlan(ctx, plan, workerDefaultAccess(cfg, db))
	return writeFreePlanApplyResult(out, n, len(plan.Orgs), err)
}

// writeFreePlanApplyResult mencetak APPLIED hanya bila semua organisasi berhasil di-re-provision; pada
// kegagalan sebagian mencetak done/total lalu mengembalikan error-nya.
func writeFreePlanApplyResult(out io.Writer, done, total int, err error) error {
	if err != nil {
		fmt.Fprintf(out, "\nGAGAL SEBAGIAN: produk Free sudah diperbarui; re-provision %d/%d organisasi sebelum error: %v\n",
			done, total, err)
		return err
	}
	fmt.Fprintf(out, "\nAPPLIED: produk Free diperbarui; re-provision %d organisasi\n", done)
	return nil
}

// RunQuotaAudit mencetak produk non-FREE dan organisasi contract/plan yang belum membawa key kuota baru
// (tenant itu tidak dibatasi/uncapped untuk key tersebut). Baca-saja, informasional.
func RunQuotaAudit(ctx context.Context, cfg config.Config, db *database.Pool, out io.Writer) error {
	registry := catalogFeatureRegistry{features: productrepo.NewFeatureRepository(db)}
	products := catalogservice.NewProductService(catalogrepo.NewProductRepository(db), registry)
	scope, err := newPlatformScopeResolver(organizationrepo.NewOrganizationRepository(db)).PlatformScope(ctx)
	if err != nil {
		return fmt.Errorf("resolve platform scope: %w", err)
	}
	var all []catalogdomain.Product
	for offset := 0; ; {
		page, total, err := products.List(ctx, scope, catalogrepo.ProductListFilter{Limit: 100, Offset: offset})
		if err != nil {
			return fmt.Errorf("list catalog products: %w", err)
		}
		all = append(all, page...)
		offset += len(page)
		if len(page) == 0 || int64(offset) >= total {
			break
		}
	}

	rows, err := db.Query(ctx, `
		SELECT o.id::text, o.slug, e.source, COALESCE(e.source_reference, ''), e.feature_key
		FROM organization_entitlements e
		JOIN organizations o ON o.id = e.organization_id
		WHERE o.type = 'customer' AND o.deleted_at IS NULL
			AND e.status = 'active' AND e.effective_from <= now()
			AND (e.effective_until IS NULL OR e.effective_until > now())
			AND o.id IN (
				SELECT organization_id FROM organization_entitlements
				WHERE source IN ('contract','plan') AND status = 'active' AND effective_from <= now()
					AND (effective_until IS NULL OR effective_until > now()))
		ORDER BY o.slug, e.feature_key`)
	if err != nil {
		return fmt.Errorf("audit entitlements: %w", err)
	}
	defer rows.Close()
	var data []AuditEntitlementRow
	for rows.Next() {
		var r AuditEntitlementRow
		if err := rows.Scan(&r.OrgID, &r.OrgSlug, &r.Source, &r.SourceReference, &r.FeatureKey); err != nil {
			return err
		}
		data = append(data, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	WriteQuotaAuditReport(out, BuildQuotaAudit(cfg.SelfServe.FreeProductSKU, all, data))
	return nil
}

// WriteQuotaAuditReport mencetak hasil audit.
func WriteQuotaAuditReport(w io.Writer, r QuotaAuditReport) {
	fmt.Fprintln(w, "AUDIT (baca-saja, informasional): key kuota baru yang belum dibawa produk/entitlement berbayar")
	fmt.Fprintf(w, "\nProduk katalog non-FREE aktif tanpa kuota lengkap: %d\n", len(r.Products))
	for _, p := range r.Products {
		fmt.Fprintf(w, "  %s (%s) belum punya: %v\n", p.SKU, p.Name, p.Missing)
	}
	fmt.Fprintf(w, "\nOrganisasi contract/plan tanpa kuota lengkap: %d\n", len(r.Orgs))
	for _, o := range r.Orgs {
		fmt.Fprintf(w, "  %s sumber=%v belum punya: %v\n", o.Slug, o.Sources, o.Missing)
	}
	fmt.Fprintln(w, "\nCatatan: tenant di atas TIDAK terkunci — untuk key yang belum dibawa mereka uncapped (tanpa batas).")
	fmt.Fprintln(w, "Tambahkan key ke produk/contract hanya bila tenant itu memang ingin dibatasi (keputusan bisnis; tidak ada back-fill otomatis).")
}
