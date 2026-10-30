package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
	organizationservice "zyad.cloud/internal/modules/organization/service"
)

// Re-provision paket Free: setelah fitur produk FREE berubah, workspace yang aksesnya bersumber `default`
// diberi nilai baru lewat ProvisionDefaultAccess (idempoten). Perencanaan di sini murni baca; penulisan
// hanya lewat ApplyFreePlan.

type FreePlanOrgRef struct{ ID, Slug string }

// freePlanStore adalah akses baca yang dibutuhkan perencanaan (dipenuhi oleh adapter DB di free_plan_runner.go).
type freePlanStore interface {
	// ListDefaultOrganizations: organisasi customer yang punya baris entitlement `default` aktif.
	ListDefaultOrganizations(ctx context.Context, at time.Time) ([]FreePlanOrgRef, error)
	// HasActiveContract: GrantDefault tidak melakukan apa pun untuk workspace seperti ini.
	HasActiveContract(ctx context.Context, organizationID string, at time.Time) (bool, error)
	ActiveDefault(ctx context.Context, organizationID string, at time.Time) ([]organizationmodel.Entitlement, error)
}

// FeatureChange: Before/After nil berarti baris tidak ada (baru ditambahkan / akan di-expire).
type FeatureChange struct {
	Key           string
	Before, After map[string]any
}

type OrgChange struct {
	Org     FreePlanOrgRef
	Changes []FeatureChange
}

type FreePlanPlan struct {
	Orgs            []OrgChange
	SkippedContract []FreePlanOrgRef
	Unchanged       int
	Warnings        []string
}

// PlanFreePlanReprovision menghitung perubahan limit per organisasi tanpa menulis apa pun. Key yang tidak ada
// di registry dilewati (sama seperti ProvisionDefaultAccess) dan dilaporkan sebagai warning.
func PlanFreePlanReprovision(
	ctx context.Context,
	store freePlanStore,
	defs map[string]catalogdomain.FeatureDef,
	features []catalogdomain.ProductFeature,
	now time.Time,
) (FreePlanPlan, error) {
	var plan FreePlanPlan
	target := map[string]map[string]any{}
	for _, f := range features {
		def, ok := defs[f.FeatureKey]
		if !ok {
			plan.Warnings = append(plan.Warnings, fmt.Sprintf("feature %s tidak ada di registry (jalankan migration terbaru); dilewati", f.FeatureKey))
			continue
		}
		limits, err := organizationservice.LimitsFromValue(def.ValueType, f.Value)
		if err != nil {
			return plan, fmt.Errorf("feature %s: %w", f.FeatureKey, err)
		}
		target[f.FeatureKey] = limits
	}

	orgs, err := store.ListDefaultOrganizations(ctx, now)
	if err != nil {
		return plan, err
	}
	for _, org := range orgs {
		hasContract, err := store.HasActiveContract(ctx, org.ID, now)
		if err != nil {
			return plan, fmt.Errorf("org %s: %w", org.Slug, err)
		}
		if hasContract {
			plan.SkippedContract = append(plan.SkippedContract, org)
			continue
		}
		current, err := store.ActiveDefault(ctx, org.ID, now)
		if err != nil {
			return plan, fmt.Errorf("org %s: %w", org.Slug, err)
		}
		changes := diffEntitlements(current, target)
		if len(changes) == 0 {
			plan.Unchanged++
			continue
		}
		plan.Orgs = append(plan.Orgs, OrgChange{Org: org, Changes: changes})
	}
	return plan, nil
}

func diffEntitlements(current []organizationmodel.Entitlement, target map[string]map[string]any) []FeatureChange {
	before := make(map[string]map[string]any, len(current))
	for _, e := range current {
		before[e.FeatureKey] = e.Limits
	}
	var changes []FeatureChange
	for key, after := range target {
		if b, ok := before[key]; !ok || !sameJSON(b, after) {
			changes = append(changes, FeatureChange{Key: key, Before: before[key], After: after})
		}
	}
	for key, b := range before {
		if _, ok := target[key]; !ok {
			changes = append(changes, FeatureChange{Key: key, Before: b})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Key < changes[j].Key })
	return changes
}

// sameJSON membandingkan limits lewat round-trip JSON (int64 dan float64 dianggap sama), seperti sameLimits.
func sameJSON(a, b map[string]any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}

// ProductFeatureChange: Before/After kosong berarti fitur tidak ada di sisi itu.
type ProductFeatureChange struct {
	Key           string
	Before, After json.RawMessage
}

// DiffProductFeatures membandingkan fitur produk FREE saat ini dengan target.
func DiffProductFeatures(current, target []catalogdomain.ProductFeature) []ProductFeatureChange {
	cur := make(map[string]json.RawMessage, len(current))
	for _, f := range current {
		cur[f.FeatureKey] = f.Value
	}
	seen := make(map[string]bool, len(target))
	var out []ProductFeatureChange
	for _, f := range target {
		seen[f.FeatureKey] = true
		if b, ok := cur[f.FeatureKey]; !ok || !sameRaw(b, f.Value) {
			out = append(out, ProductFeatureChange{Key: f.FeatureKey, Before: b, After: f.Value})
		}
	}
	for _, f := range current {
		if !seen[f.FeatureKey] {
			out = append(out, ProductFeatureChange{Key: f.FeatureKey, Before: f.Value})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func sameRaw(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return bytes.Equal(a, b)
	}
	return fmt.Sprint(x) == fmt.Sprint(y)
}

// ApplyFreePlan memanggil ProvisionDefaultAccess untuk organisasi yang berubah. Pemanggil harus sudah
// memperbarui produk FREE; provisioner membaca fitur dari produk itu.
func ApplyFreePlan(ctx context.Context, plan FreePlanPlan, provisioner DefaultAccessProvisioner) (int, error) {
	done := 0
	for _, oc := range plan.Orgs {
		if err := provisioner.ProvisionDefaultAccess(ctx, oc.Org.ID); err != nil {
			return done, fmt.Errorf("org %s: provision default: %w", oc.Org.Slug, err)
		}
		done++
	}
	return done, nil
}

// WriteFreePlanReport mencetak ringkasan; apply=false menandai laporan DRY-RUN (tidak ada yang ditulis).
func WriteFreePlanReport(w io.Writer, productDiff []ProductFeatureChange, plan FreePlanPlan, apply bool) {
	if apply {
		fmt.Fprintln(w, "MODE: APPLY")
	} else {
		fmt.Fprintln(w, "MODE: DRY-RUN (tidak ada yang ditulis; tambahkan --apply untuk menjalankan)")
	}
	for _, warn := range plan.Warnings {
		fmt.Fprintln(w, "WARNING:", warn)
	}
	fmt.Fprintf(w, "\nProduk FREE: %d perubahan fitur\n", len(productDiff))
	for _, d := range productDiff {
		fmt.Fprintf(w, "  %-40s %s -> %s\n", d.Key, rawOrNone(d.Before), rawOrNone(d.After))
	}
	fmt.Fprintf(w, "\nOrganisasi terdampak: %d (tanpa perubahan: %d, dilewati karena punya contract aktif: %d)\n",
		len(plan.Orgs), plan.Unchanged, len(plan.SkippedContract))
	for _, oc := range plan.Orgs {
		fmt.Fprintf(w, "  %s (%s)\n", oc.Org.Slug, oc.Org.ID)
		for _, c := range oc.Changes {
			fmt.Fprintf(w, "    %-40s %s -> %s\n", c.Key, limitsText(c.Before), limitsText(c.After))
		}
	}
}

func rawOrNone(v json.RawMessage) string {
	if len(v) == 0 {
		return "(none)"
	}
	return string(v)
}

func limitsText(l map[string]any) string {
	if l == nil {
		return "(none)"
	}
	b, _ := json.Marshal(l)
	return string(b)
}
