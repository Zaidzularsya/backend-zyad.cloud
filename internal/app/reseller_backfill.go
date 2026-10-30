package app

import (
	"context"
	"fmt"
	"io"
	"time"

	organizationmodel "zyad.cloud/internal/modules/organization/model"
	organizationrepo "zyad.cloud/internal/modules/organization/repository"
	organizationservice "zyad.cloud/internal/modules/organization/service"
	"zyad.cloud/internal/platform/database"
)

// ResellerOrgRef adalah workspace yang memegang entitlement landing.page_activation dari contract.
// ContractRef dipakai sebagai source_reference baris bundle supaya ikut dicabut bersama contract-nya.
type ResellerOrgRef struct {
	ID, Slug, ContractRef string
}

// ResellerBundleStore adalah pembacaan untuk perencanaan (read-only).
type ResellerBundleStore interface {
	ListResellers(ctx context.Context, at time.Time) ([]ResellerOrgRef, error)
	ActiveFeatureKeys(ctx context.Context, organizationID string, keys []string, at time.Time) (map[string]bool, error)
}

// ResellerBundleWriter menulis fitur bundle yang masih kurang untuk satu workspace.
type ResellerBundleWriter interface {
	GrantMissing(ctx context.Context, ref ResellerOrgRef, grants []organizationservice.FeatureGrant, at time.Time) error
}

type ResellerBundleChange struct {
	Org     ResellerOrgRef
	Missing []organizationservice.FeatureGrant
}

type ResellerBundlePlan struct {
	Changes   []ResellerBundleChange
	Unchanged int
}

// PlanResellerBundle menghitung fitur bundle yang belum aktif di tiap reseller. Fitur yang sudah aktif
// dari sumber mana pun (contract, default, override platform) tidak ditimpa.
func PlanResellerBundle(ctx context.Context, store ResellerBundleStore, at time.Time) (ResellerBundlePlan, error) {
	resellers, err := store.ListResellers(ctx, at)
	if err != nil {
		return ResellerBundlePlan{}, err
	}
	keys := make([]string, len(resellerBundleSpec))
	for i, f := range resellerBundleSpec {
		keys[i] = f.Key
	}
	var plan ResellerBundlePlan
	for _, ref := range resellers {
		active, err := store.ActiveFeatureKeys(ctx, ref.ID, keys, at)
		if err != nil {
			return ResellerBundlePlan{}, fmt.Errorf("read active features of %s: %w", ref.Slug, err)
		}
		var missing []organizationservice.FeatureGrant
		for _, f := range resellerBundleSpec {
			if !active[f.Key] {
				missing = append(missing, organizationservice.FeatureGrant{Key: f.Key, ValueType: f.ValueType, Value: []byte(f.Value)})
			}
		}
		if len(missing) == 0 {
			plan.Unchanged++
			continue
		}
		plan.Changes = append(plan.Changes, ResellerBundleChange{Org: ref, Missing: missing})
	}
	return plan, nil
}

func WriteResellerBundleReport(out io.Writer, plan ResellerBundlePlan, apply bool) {
	mode := "DRY-RUN (tidak ada yang ditulis; tambahkan --apply untuk menjalankan)"
	if apply {
		mode = "APPLY"
	}
	fmt.Fprintf(out, "MODE: %s\n\nReseller terdampak: %d (sudah lengkap: %d)\n", mode, len(plan.Changes), plan.Unchanged)
	for _, c := range plan.Changes {
		fmt.Fprintf(out, "  %s (%s) contract=%s\n", c.Org.Slug, c.Org.ID, c.Org.ContractRef)
		for _, g := range c.Missing {
			fmt.Fprintf(out, "    %-32s (none) -> %s\n", g.Key, g.Value)
		}
	}
}

// ApplyResellerBundle menulis fitur yang kurang; mengembalikan jumlah workspace yang ditulis. Idempoten:
// menjalankan ulang setelah berhasil menghasilkan rencana kosong.
func ApplyResellerBundle(ctx context.Context, plan ResellerBundlePlan, w ResellerBundleWriter, at time.Time) (int, error) {
	done := 0
	for _, c := range plan.Changes {
		if err := w.GrantMissing(ctx, c.Org, c.Missing, at); err != nil {
			return done, fmt.Errorf("grant bundle to %s: %w", c.Org.Slug, err)
		}
		done++
	}
	return done, nil
}

type dbResellerStore struct{ db *database.Pool }

func (s dbResellerStore) ListResellers(ctx context.Context, at time.Time) ([]ResellerOrgRef, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT ON (o.id) o.id::text, o.slug, e.source_reference
		FROM organization_entitlements e
		JOIN organizations o ON o.id = e.organization_id AND o.type = 'customer' AND o.deleted_at IS NULL
		WHERE e.feature_key = $1 AND e.source = 'contract' AND e.status = 'active'
			AND e.source_reference IS NOT NULL AND e.source_reference <> ''
			AND e.effective_from <= $2 AND (e.effective_until IS NULL OR e.effective_until > $2)
			AND COALESCE(e.limits->>'value', e.limits->>'enabled') = 'true'
		ORDER BY o.id, e.effective_from`, featurePageActivation, at)
	if err != nil {
		return nil, fmt.Errorf("list resellers: %w", err)
	}
	defer rows.Close()
	var out []ResellerOrgRef
	for rows.Next() {
		var r ResellerOrgRef
		if err := rows.Scan(&r.ID, &r.Slug, &r.ContractRef); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s dbResellerStore) ActiveFeatureKeys(ctx context.Context, organizationID string, keys []string, at time.Time) (map[string]bool, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT feature_key FROM organization_entitlements
		WHERE organization_id = $1::uuid AND status = 'active' AND feature_key = ANY($2)
			AND effective_from <= $3 AND (effective_until IS NULL OR effective_until > $3)`, organizationID, keys, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out[k] = true
	}
	return out, rows.Err()
}

type dbResellerWriter struct {
	entitlements *organizationrepo.EntitlementRepository
}

func (w dbResellerWriter) GrantMissing(ctx context.Context, ref ResellerOrgRef, grants []organizationservice.FeatureGrant, at time.Time) error {
	for _, g := range grants {
		limits, err := organizationservice.LimitsFromValue(g.ValueType, g.Value)
		if err != nil {
			return fmt.Errorf("feature %s: %w", g.Key, err)
		}
		if _, err := w.entitlements.Upsert(ctx, organizationrepo.UpsertEntitlementParams{
			OrganizationID: ref.ID, FeatureKey: g.Key, Source: organizationmodel.EntitlementSourceContract,
			SourceReference: ref.ContractRef, Status: organizationmodel.EntitlementStatusActive,
			Limits: limits, EffectiveFrom: at, Reason: "reseller bundle backfill",
		}); err != nil {
			return err
		}
	}
	return nil
}

// RunResellerBundle melengkapi bundle reseller untuk workspace yang sudah memegang
// landing.page_activation. apply=false (default CLI) hanya mencetak rencana dan tidak menulis apa pun.
func RunResellerBundle(ctx context.Context, db *database.Pool, apply bool, out io.Writer) error {
	now := time.Now().UTC()
	plan, err := PlanResellerBundle(ctx, dbResellerStore{db: db}, now)
	if err != nil {
		return err
	}
	WriteResellerBundleReport(out, plan, apply)
	if !apply {
		return nil
	}
	n, err := ApplyResellerBundle(ctx, plan, dbResellerWriter{entitlements: organizationrepo.NewEntitlementRepository(db)}, now)
	if err != nil {
		fmt.Fprintf(out, "\nGAGAL SEBAGIAN: %d/%d workspace selesai: %v\n", n, len(plan.Changes), err)
		return err
	}
	fmt.Fprintf(out, "\nAPPLIED: bundle reseller dilengkapi di %d workspace\n", n)
	return nil
}
