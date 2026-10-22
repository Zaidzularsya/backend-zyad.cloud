package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"net/http"
	coreerrors "zyad.cloud/internal/core/errors"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
)

// FeatureDefReader reads feature registry definitions by key.
type FeatureDefReader interface {
	FindByKeys(ctx context.Context, keys []string) (map[string]catalogdomain.FeatureDef, error)
}

// QuotaEnforcer checks that an organization still has room for one more unit
// of a quota feature, counting usage through the registered QuotaMeter.
type QuotaEnforcer struct {
	guard    *EntitlementGuard
	features FeatureDefReader
	meters   *QuotaMeterRegistry
	now      func() time.Time
}

func NewQuotaEnforcer(guard *EntitlementGuard, features FeatureDefReader, meters *QuotaMeterRegistry, now func() time.Time) *QuotaEnforcer {
	if now == nil {
		now = time.Now
	}
	return &QuotaEnforcer{guard: guard, features: features, meters: meters, now: now}
}

// RequireRoom returns QUOTA_EXCEEDED when usage already reached the limit,
// FEATURE_NOT_ENABLED when the feature is not entitled, and nil otherwise.
// Platform organizations are bypassed by the underlying guard.
func (e *QuotaEnforcer) RequireRoom(ctx context.Context, organizationID, featureKey string) error {
	featureKey = strings.TrimSpace(featureKey)
	// Platform organizations are exempt from every limit: return before any
	// meter/registry/period work so misconfiguration or COUNT cost never applies.
	isPlatform, err := e.guard.IsPlatformOrganization(ctx, organizationID)
	if err != nil {
		return err
	}
	if isPlatform {
		return nil
	}
	meter, ok := e.meters.Get(featureKey)
	if !ok {
		return quotaConfigError(fmt.Sprintf("quota meter for %s is not registered", featureKey))
	}
	defs, err := e.features.FindByKeys(ctx, []string{featureKey})
	if err != nil {
		return err
	}
	def, ok := defs[featureKey]
	if !ok {
		return quotaConfigError(fmt.Sprintf("feature %s is not in the registry", featureKey))
	}
	periodStart, err := PeriodStart(def.ResetStrategy, e.now())
	if err != nil {
		return quotaConfigError(err.Error())
	}
	used, err := meter.Used(ctx, organizationID, periodStart)
	if err != nil {
		return err
	}
	return e.guard.RequireQuotaValue(ctx, organizationID, featureKey, "limit", used, 1)
}

func quotaConfigError(msg string) error {
	return coreerrors.New("QUOTA_ENFORCER_MISCONFIGURED", msg, http.StatusInternalServerError)
}
