package app

import (
	"encoding/json"
	"reflect"
	"testing"

	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
)

func auditProduct(sku string, active bool, keys ...string) catalogdomain.Product {
	p := catalogdomain.Product{SKU: sku, Name: sku, IsActive: active}
	for _, k := range keys {
		p.Features = append(p.Features, catalogdomain.ProductFeature{FeatureKey: k, Value: json.RawMessage(`1`)})
	}
	return p
}

func TestBuildQuotaAudit(t *testing.T) {
	all := append([]string{}, newQuotaFeatureKeys...)
	products := []catalogdomain.Product{
		auditProduct("FREE", true, "crm.enabled"),                 // Free is never audited
		auditProduct("PRO", true, "crm.enabled", "crm.max_leads"), // missing 7
		auditProduct("FULL", true, all...),                        // complete
		auditProduct("OLD", false, "crm.enabled"),                 // inactive: ignored
	}
	rows := []AuditEntitlementRow{
		{OrgID: "1", OrgSlug: "acme", Source: "contract", SourceReference: "c-1", FeatureKey: "crm.enabled"},
		{OrgID: "1", OrgSlug: "acme", Source: "contract", SourceReference: "c-1", FeatureKey: "crm.max_leads"},
		{OrgID: "1", OrgSlug: "acme", Source: "plan", SourceReference: "", FeatureKey: "crm.max_deals"},
	}
	for _, k := range all {
		rows = append(rows, AuditEntitlementRow{OrgID: "2", OrgSlug: "full", Source: "contract", SourceReference: "c-2", FeatureKey: k})
	}
	report := BuildQuotaAudit("FREE", products, rows)

	if len(report.Products) != 1 || report.Products[0].SKU != "PRO" || len(report.Products[0].Missing) != 7 {
		t.Fatalf("products = %+v", report.Products)
	}
	if len(report.Orgs) != 1 || report.Orgs[0].Slug != "acme" {
		t.Fatalf("orgs = %+v", report.Orgs)
	}
	org := report.Orgs[0]
	if !reflect.DeepEqual(org.Sources, []string{"contract:c-1", "plan"}) {
		t.Errorf("sources = %v", org.Sources)
	}
	// acme holds max_leads and max_deals (union across sources); 6 remain missing.
	if len(org.Missing) != 6 {
		t.Errorf("missing = %v", org.Missing)
	}
	for _, k := range org.Missing {
		if k == "crm.max_leads" || k == "crm.max_deals" {
			t.Errorf("%s should not be missing", k)
		}
	}
}

func TestBuildQuotaAuditSkipsDefaultOnlyOrgs(t *testing.T) {
	rows := []AuditEntitlementRow{{OrgID: "3", OrgSlug: "free-only", Source: "default", FeatureKey: "crm.enabled"}}
	if r := BuildQuotaAudit("FREE", nil, rows); len(r.Orgs) != 0 {
		t.Fatalf("orgs = %+v", r.Orgs)
	}
}
