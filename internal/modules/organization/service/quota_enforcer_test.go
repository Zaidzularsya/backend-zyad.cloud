package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	organizationmodel "zyad.cloud/internal/modules/organization/model"
)

type fakeMeter struct {
	used        int64
	err         error
	gotPeriod   *time.Time
	calledCount *int
}

func (m fakeMeter) Used(_ context.Context, _ string, periodStart *time.Time) (int64, error) {
	if m.calledCount != nil {
		*m.calledCount++
	}
	return m.used, m.err
}

type fakeFeatureDefs struct {
	defs map[string]catalogdomain.FeatureDef
}

func (f fakeFeatureDefs) FindByKeys(_ context.Context, keys []string) (map[string]catalogdomain.FeatureDef, error) {
	out := map[string]catalogdomain.FeatureDef{}
	for _, k := range keys {
		if d, ok := f.defs[k]; ok {
			out[k] = d
		}
	}
	return out, nil
}

func newTestEnforcer(used int64, limit int64, evalErr error, platform bool) *QuotaEnforcer {
	var opts []EntitlementGuardOption
	if platform {
		opts = append(opts, WithOrganizationTypeResolver(stubOrganizationTypeResolver{
			organization: organizationmodel.Organization{Type: coretenant.OrganizationTypePlatform},
		}))
	} else {
		opts = append(opts, WithOrganizationTypeResolver(stubOrganizationTypeResolver{
			organization: organizationmodel.Organization{Type: coretenant.OrganizationTypeCustomer},
		}))
	}
	guard := NewEntitlementGuard(stubEntitlementEvaluator{
		entitlement: organizationmodel.Entitlement{
			FeatureKey: "crm.max_leads",
			Limits:     map[string]any{"limit": limit},
		},
		err: evalErr,
	}, opts...)
	meters := NewQuotaMeterRegistry()
	meters.Register("crm.max_leads", fakeMeter{used: used})
	defs := fakeFeatureDefs{defs: map[string]catalogdomain.FeatureDef{
		"crm.max_leads": {Key: "crm.max_leads", ValueType: "integer", ResetStrategy: "never", IsActive: true},
	}}
	present := fakePresence{ent: organizationmodel.Entitlement{FeatureKey: "crm.max_leads", Limits: map[string]any{"limit": limit}}}
	return NewQuotaEnforcer(guard, present, defs, meters, time.Now)
}

func TestRequireRoomAllowsBelowLimit(t *testing.T) {
	e := newTestEnforcer(49, 50, nil, false)
	if err := e.RequireRoom(context.Background(), "org-1", "crm.max_leads"); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestRequireRoomRejectsAtLimitWithQuotaExceeded(t *testing.T) {
	e := newTestEnforcer(50, 50, nil, false)
	err := e.RequireRoom(context.Background(), "org-1", "crm.max_leads")
	assertAppErrorCode(t, err, "QUOTA_EXCEEDED")
}

func TestRequireRoomPlatformOrgBypassesLimit(t *testing.T) {
	e := newTestEnforcer(9999, 50, coreerrors.New("ORGANIZATION_FEATURE_NOT_ENTITLED", "x", http.StatusForbidden), true)
	if err := e.RequireRoom(context.Background(), "org-1", "crm.max_leads"); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestRequireRoomHiddenFeatureReturnsFeatureNotEnabled(t *testing.T) {
	e := newTestEnforcer(0, 50, coreerrors.New("ORGANIZATION_FEATURE_NOT_ENTITLED", "x", http.StatusForbidden), false)
	err := e.RequireRoom(context.Background(), "org-1", "crm.max_leads")
	assertAppErrorCode(t, err, "FEATURE_NOT_ENABLED")
}

func TestRequireRoomUnknownMeterReturnsError(t *testing.T) {
	e := newTestEnforcer(0, 50, nil, false)
	err := e.RequireRoom(context.Background(), "org-1", "crm.max_unknown")
	if err == nil {
		t.Fatal("want error")
	}
	assertAppErrorCode(t, err, "QUOTA_ENFORCER_MISCONFIGURED")
}

func platformGuard() *EntitlementGuard {
	return NewEntitlementGuard(stubEntitlementEvaluator{}, WithOrganizationTypeResolver(stubOrganizationTypeResolver{
		organization: organizationmodel.Organization{Type: coretenant.OrganizationTypePlatform},
	}))
}

func TestRequireRoomPlatformOrgSkipsRegistries(t *testing.T) {
	e := NewQuotaEnforcer(platformGuard(), fakePresence{}, fakeFeatureDefs{}, NewQuotaMeterRegistry(), time.Now)
	if err := e.RequireRoom(context.Background(), "org-1", "crm.max_leads"); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestRequireRoomPlatformOrgDoesNotCallMeter(t *testing.T) {
	calls := 0
	meters := NewQuotaMeterRegistry()
	meters.Register("crm.max_leads", fakeMeter{used: 9999, calledCount: &calls})
	defs := fakeFeatureDefs{defs: map[string]catalogdomain.FeatureDef{
		"crm.max_leads": {Key: "crm.max_leads", ResetStrategy: "yearly"},
	}}
	e := NewQuotaEnforcer(platformGuard(), fakePresence{}, defs, meters, time.Now)
	if err := e.RequireRoom(context.Background(), "org-1", "crm.max_leads"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if calls != 0 {
		t.Fatalf("meter called %d times, want 0", calls)
	}
}

// fakePresence answers FindEffective: absent => pgx.ErrNoRows.
type fakePresence struct {
	ent    organizationmodel.Entitlement
	absent bool
	calls  *int
}

func (f fakePresence) FindEffective(_ context.Context, _ string, _ string, _ time.Time) (organizationmodel.Entitlement, error) {
	if f.calls != nil {
		*f.calls++
	}
	if f.absent {
		return organizationmodel.Entitlement{}, pgx.ErrNoRows
	}
	return f.ent, nil
}

var notEntitledErr = coreerrors.New("ORGANIZATION_FEATURE_NOT_ENTITLED", "organization feature is not enabled", http.StatusForbidden)

func customerEnforcer(key string, presence fakePresence, eval stubEntitlementEvaluator, used int64) *QuotaEnforcer {
	guard := NewEntitlementGuard(eval, WithOrganizationTypeResolver(stubOrganizationTypeResolver{
		organization: organizationmodel.Organization{Type: coretenant.OrganizationTypeCustomer},
	}))
	meters := NewQuotaMeterRegistry()
	meters.Register(key, fakeMeter{used: used})
	defs := fakeFeatureDefs{defs: map[string]catalogdomain.FeatureDef{
		key: {Key: key, ValueType: "integer", ResetStrategy: "never", IsActive: true},
	}}
	return NewQuotaEnforcer(guard, presence, defs, meters, time.Now)
}

func TestRequireRoomAbsentNewKeyIsUnlimited(t *testing.T) {
	for _, key := range UncappedWhenAbsentQuotaKeys() {
		e := customerEnforcer(key, fakePresence{absent: true}, stubEntitlementEvaluator{err: notEntitledErr}, 1_000_000)
		if err := e.RequireRoom(context.Background(), "org-1", key); err != nil {
			t.Fatalf("%s: err = %v, want nil (uncapped)", key, err)
		}
	}
}

func TestRequireRoomExplicitlyDisabledNewKeyIsNotEnabled(t *testing.T) {
	disabled := organizationmodel.Entitlement{FeatureKey: "crm.max_deals", Limits: map[string]any{"value": false, "enabled": false}}
	e := customerEnforcer("crm.max_deals", fakePresence{ent: disabled}, stubEntitlementEvaluator{err: notEntitledErr}, 0)
	assertAppErrorCode(t, e.RequireRoom(context.Background(), "org-1", "crm.max_deals"), "FEATURE_NOT_ENABLED")
}

func TestRequireRoomNewKeyWithLimitIsEnforced(t *testing.T) {
	row := organizationmodel.Entitlement{FeatureKey: "domain.max_domains", Limits: map[string]any{"limit": int64(1)}}
	eval := stubEntitlementEvaluator{entitlement: row}
	full := customerEnforcer("domain.max_domains", fakePresence{ent: row}, eval, 1)
	assertAppErrorCode(t, full.RequireRoom(context.Background(), "org-1", "domain.max_domains"), "QUOTA_EXCEEDED")
	room := customerEnforcer("domain.max_domains", fakePresence{ent: row}, eval, 0)
	if err := room.RequireRoom(context.Background(), "org-1", "domain.max_domains"); err != nil {
		t.Fatalf("err = %v", err)
	}
}

func TestRequireRoomLegacyKeyAbsentStillNotEnabled(t *testing.T) {
	for _, key := range []string{"crm.max_contacts", "landing.max_pages", "users.max_users", "storage.max_bytes"} {
		if IsUncappedWhenAbsent(key) {
			t.Fatalf("%s must keep its original semantics", key)
		}
		calls := 0
		e := customerEnforcer(key, fakePresence{absent: true, calls: &calls}, stubEntitlementEvaluator{err: notEntitledErr}, 0)
		assertAppErrorCode(t, e.RequireRoom(context.Background(), "org-1", key), "FEATURE_NOT_ENABLED")
		if calls != 0 {
			t.Fatalf("%s: presence lookup called %d times, want 0", key, calls)
		}
	}
}
