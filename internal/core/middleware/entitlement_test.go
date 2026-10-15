package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"

	"github.com/gin-gonic/gin"
)

type fakeEntitlementChecker struct{ enabled map[string]bool }

func (f fakeEntitlementChecker) RequireFeature(_ context.Context, _ string, featureKey string) error {
	if f.enabled[featureKey] {
		return nil
	}
	return coreerrors.New("FEATURE_NOT_ENTITLED", "feature "+featureKey+" is not enabled", http.StatusForbidden)
}

func performAnyEntitlement(t *testing.T, checker EntitlementChecker, keys ...string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET(
		"/x",
		setTenantContextMiddleware(mustTenantContextWith(
			t,
			coretenant.OrganizationTypeCustomer,
			coretenant.OrganizationStatusActive,
			coretenant.ResolutionSourceCustomDomain,
		)),
		RequireAnyEntitlement(checker, keys...),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	return rec.Code
}

func TestRequireAnyEntitlementPassesWhenOneFeatureEnabled(t *testing.T) {
	checker := fakeEntitlementChecker{enabled: map[string]bool{"receivable.enabled": true}}
	if code := performAnyEntitlement(t, checker, "crm.enabled", "receivable.enabled"); code != http.StatusOK {
		t.Fatalf("one enabled → %d", code)
	}
}

func TestRequireAnyEntitlementRejectsWhenNoneEnabled(t *testing.T) {
	checker := fakeEntitlementChecker{enabled: map[string]bool{}}
	if code := performAnyEntitlement(t, checker, "crm.enabled", "receivable.enabled"); code != http.StatusForbidden {
		t.Fatalf("none enabled → %d", code)
	}
}

func TestRequireAnyEntitlementFailsClosedWithoutChecker(t *testing.T) {
	if code := performAnyEntitlement(t, nil, "crm.enabled"); code != http.StatusInternalServerError {
		t.Fatalf("nil checker → %d", code)
	}
}

func TestRequireAnyEntitlementFailsClosedWithoutKeys(t *testing.T) {
	checker := fakeEntitlementChecker{enabled: map[string]bool{"crm.enabled": true}}
	if code := performAnyEntitlement(t, checker); code != http.StatusForbidden {
		t.Fatalf("no keys → %d", code)
	}
}
