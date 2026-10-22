package service

import (
	"context"
	"testing"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/organization/dto"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
	"zyad.cloud/internal/modules/organization/repository"
)

type fakeEntitlementLister struct {
	items []organizationmodel.Entitlement
}

func (f fakeEntitlementLister) ListEffective(_ context.Context, _ string, filter repository.EntitlementListFilter, _ time.Time) ([]organizationmodel.Entitlement, int64, error) {
	total := int64(len(f.items))
	start := filter.Offset
	if start > len(f.items) {
		start = len(f.items)
	}
	end := len(f.items)
	if filter.Limit > 0 && start+filter.Limit < end {
		end = start + filter.Limit
	}
	return f.items[start:end], total, nil
}

type fakeFeatureDefLister struct{ defs []catalogdomain.FeatureDef }

func (f fakeFeatureDefLister) ListActive(context.Context) ([]catalogdomain.FeatureDef, error) {
	return f.defs, nil
}

func summaryFor(t *testing.T, platform bool, defs []catalogdomain.FeatureDef, ents []organizationmodel.Entitlement, used int64) []dto.EntitlementSummaryItem {
	t.Helper()
	typ := coretenant.OrganizationTypeCustomer
	if platform {
		typ = coretenant.OrganizationTypePlatform
	}
	meters := NewQuotaMeterRegistry()
	meters.Register("crm.max_leads", fakeMeter{used: used})
	svc := NewEntitlementSummaryService(
		fakeEntitlementLister{items: ents},
		fakeFeatureDefLister{defs: defs},
		meters,
		stubOrganizationTypeResolver{organization: organizationmodel.Organization{Type: typ}},
		time.Now,
	)
	items, err := svc.Summary(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	return items
}

var (
	boolDef = catalogdomain.FeatureDef{Key: "crm.enabled", Module: "crm", Name: "CRM", ValueType: "boolean", IsActive: true}
	intDef  = catalogdomain.FeatureDef{Key: "crm.max_leads", Module: "crm", Name: "Lead", ValueType: "integer", Unit: "lead", ResetStrategy: "never", IsActive: true}
)

func ent(key string, limits map[string]any) organizationmodel.Entitlement {
	return organizationmodel.Entitlement{FeatureKey: key, Limits: limits}
}

func TestSummaryBooleanTrueIsUnlimited(t *testing.T) {
	items := summaryFor(t, false, []catalogdomain.FeatureDef{boolDef},
		[]organizationmodel.Entitlement{ent("crm.enabled", map[string]any{"value": true, "enabled": true})}, 0)
	if len(items) != 1 || items[0].State != "unlimited" {
		t.Fatalf("items = %+v", items)
	}
}

func TestSummaryBooleanFalseIsHidden(t *testing.T) {
	items := summaryFor(t, false, []catalogdomain.FeatureDef{boolDef},
		[]organizationmodel.Entitlement{ent("crm.enabled", map[string]any{"value": false, "enabled": false})}, 0)
	if len(items) != 1 || items[0].State != "hidden" {
		t.Fatalf("items = %+v", items)
	}
}

func TestSummaryIntegerIsQuotaWithUsedAndRemaining(t *testing.T) {
	items := summaryFor(t, false, []catalogdomain.FeatureDef{intDef},
		[]organizationmodel.Entitlement{ent("crm.max_leads", map[string]any{"value": int64(50), "limit": int64(50)})}, 12)
	it := items[0]
	if it.State != "quota" || it.Limit == nil || *it.Limit != 50 || it.Used == nil || *it.Used != 12 ||
		it.Remaining == nil || *it.Remaining != 38 || it.AtLimit || it.Unit != "lead" || it.ResetStrategy != "never" {
		t.Fatalf("item = %+v", it)
	}
}

func TestSummaryAtLimitFlag(t *testing.T) {
	items := summaryFor(t, false, []catalogdomain.FeatureDef{intDef},
		[]organizationmodel.Entitlement{ent("crm.max_leads", map[string]any{"limit": float64(50)})}, 50)
	it := items[0]
	if !it.AtLimit || it.Remaining == nil || *it.Remaining != 0 {
		t.Fatalf("item = %+v", it)
	}
}

func TestSummaryMissingEntitlementIsHidden(t *testing.T) {
	// crm.max_contacts is a pre-existing key: absent => hidden (crm.max_leads would be uncapped).
	contactsDef := catalogdomain.FeatureDef{Key: "crm.max_contacts", Module: "crm", ValueType: "integer", ResetStrategy: "never", IsActive: true}
	items := summaryFor(t, false, []catalogdomain.FeatureDef{boolDef, contactsDef}, nil, 0)
	if len(items) != 2 || items[0].State != "hidden" || items[1].State != "hidden" {
		t.Fatalf("items = %+v", items)
	}
	if items[1].Limit != nil || items[1].Used != nil {
		t.Fatalf("hidden item must not carry numbers: %+v", items[1])
	}
}

func TestSummaryPlatformOrgAllUnlimited(t *testing.T) {
	items := summaryFor(t, true, []catalogdomain.FeatureDef{intDef, boolDef}, nil, 0)
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	for _, it := range items {
		if it.State != "unlimited" || it.Limit != nil {
			t.Fatalf("item = %+v", it)
		}
	}
}

func TestSummarySortedByModuleThenKey(t *testing.T) {
	a := catalogdomain.FeatureDef{Key: "z.b", Module: "z", ValueType: "boolean", IsActive: true}
	b := catalogdomain.FeatureDef{Key: "a.y", Module: "a", ValueType: "boolean", IsActive: true}
	c := catalogdomain.FeatureDef{Key: "a.x", Module: "a", ValueType: "boolean", IsActive: true}
	items := summaryFor(t, true, []catalogdomain.FeatureDef{a, b, c}, nil, 0)
	got := items[0].FeatureKey + items[1].FeatureKey + items[2].FeatureKey
	if got != "a.xa.yz.b" {
		t.Fatalf("order = %s", got)
	}
}

var dealsDef = catalogdomain.FeatureDef{Key: "crm.max_deals", Module: "crm", Name: "Deal", ValueType: "integer", Unit: "deal", ResetStrategy: "never", IsActive: true}

func TestSummaryAbsentNewKeyIsUnlimited(t *testing.T) {
	items := summaryFor(t, false, []catalogdomain.FeatureDef{dealsDef}, nil, 0)
	it := items[0]
	if it.State != "unlimited" || it.Limit != nil || it.Used != nil || it.Remaining != nil || it.AtLimit {
		t.Fatalf("item = %+v", it)
	}
}

func TestSummaryExplicitlyDisabledNewKeyIsHidden(t *testing.T) {
	items := summaryFor(t, false, []catalogdomain.FeatureDef{dealsDef},
		[]organizationmodel.Entitlement{ent("crm.max_deals", map[string]any{"value": false, "enabled": false})}, 0)
	if items[0].State != "hidden" {
		t.Fatalf("item = %+v", items[0])
	}
}

func TestSummaryAbsentLegacyKeyStaysHidden(t *testing.T) {
	contactsDef := catalogdomain.FeatureDef{Key: "crm.max_contacts", Module: "crm", ValueType: "integer", ResetStrategy: "never", IsActive: true}
	pagesDef := catalogdomain.FeatureDef{Key: "landing.max_pages", Module: "landing", ValueType: "integer", ResetStrategy: "never", IsActive: true}
	items := summaryFor(t, false, []catalogdomain.FeatureDef{contactsDef, pagesDef}, nil, 0)
	for _, it := range items {
		if it.State != "hidden" {
			t.Fatalf("item = %+v", it)
		}
	}
}
