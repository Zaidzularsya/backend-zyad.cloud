package app

import (
	"sort"
	"strings"

	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
)

// newQuotaFeatureKeys adalah key kuota baru (migration 000158/000159). Produk berbayar/contract yang tidak
// membawa satu pun dari key ini akan menerima FEATURE_NOT_ENABLED saat membuat data terkait.
var newQuotaFeatureKeys = []string{
	"crm.max_leads", "crm.max_companies", "crm.max_deals", "crm.max_pipelines",
	"crm.max_quotations_per_month", "crm.max_sales_orders_per_month",
	"receivable.max_invoices_per_month", "domain.max_domains",
}

// AuditEntitlementRow: satu baris entitlement aktif organisasi customer (hasil query baca-saja).
type AuditEntitlementRow struct {
	OrgID, OrgSlug, Source, SourceReference, FeatureKey string
}

type AuditProductGap struct {
	SKU, Name string
	Missing   []string
}

type AuditOrgGap struct {
	ID, Slug string
	Sources  []string // "contract:<ref>" / "plan" ...
	Missing  []string
}

type QuotaAuditReport struct {
	Products []AuditProductGap
	Orgs     []AuditOrgGap
}

func missingQuotaKeys(have map[string]bool) []string {
	var missing []string
	for _, k := range newQuotaFeatureKeys {
		if !have[k] {
			missing = append(missing, k)
		}
	}
	return missing
}

// BuildQuotaAudit mengelompokkan produk non-FREE aktif dan organisasi ber-sumber contract/plan yang belum
// memiliki key kuota baru. Murni; tidak menulis dan tidak mengisi otomatis (itu keputusan bisnis).
func BuildQuotaAudit(freeSKU string, products []catalogdomain.Product, rows []AuditEntitlementRow) QuotaAuditReport {
	var report QuotaAuditReport
	for _, p := range products {
		if p.SKU == freeSKU || !p.IsActive {
			continue
		}
		have := map[string]bool{}
		for _, f := range p.Features {
			have[f.FeatureKey] = true
		}
		if missing := missingQuotaKeys(have); len(missing) > 0 {
			report.Products = append(report.Products, AuditProductGap{SKU: p.SKU, Name: p.Name, Missing: missing})
		}
	}

	type orgAgg struct {
		slug    string
		keys    map[string]bool
		sources map[string]bool
		paid    bool
	}
	orgs := map[string]*orgAgg{}
	for _, r := range rows {
		o := orgs[r.OrgID]
		if o == nil {
			o = &orgAgg{slug: r.OrgSlug, keys: map[string]bool{}, sources: map[string]bool{}}
			orgs[r.OrgID] = o
		}
		o.keys[r.FeatureKey] = true
		if r.Source == "contract" || r.Source == "plan" {
			o.paid = true
			label := r.Source
			if r.SourceReference != "" {
				label += ":" + r.SourceReference
			}
			o.sources[label] = true
		}
	}
	for id, o := range orgs {
		if !o.paid {
			continue
		}
		if missing := missingQuotaKeys(o.keys); len(missing) > 0 {
			sources := make([]string, 0, len(o.sources))
			for s := range o.sources {
				sources = append(sources, s)
			}
			sort.Strings(sources)
			report.Orgs = append(report.Orgs, AuditOrgGap{ID: id, Slug: o.slug, Sources: sources, Missing: missing})
		}
	}
	sort.Slice(report.Orgs, func(i, j int) bool { return strings.Compare(report.Orgs[i].Slug, report.Orgs[j].Slug) < 0 })
	sort.Slice(report.Products, func(i, j int) bool { return report.Products[i].SKU < report.Products[j].SKU })
	return report
}
