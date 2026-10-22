package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

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
	return NewQuotaEnforcer(guard, defs, meters, time.Now)
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
	var appErr *coreerrors.AppError
	if errors.As(err, &appErr) && appErr.Code == "QUOTA_EXCEEDED" {
		t.Fatalf("unexpected quota error: %v", err)
	}
}
