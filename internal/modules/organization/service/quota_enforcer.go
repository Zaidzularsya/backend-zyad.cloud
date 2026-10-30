package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	catalogdomain "zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/organization/model"
)

// FeatureDefReader reads feature registry definitions by key.
type FeatureDefReader interface {
	FindByKeys(ctx context.Context, keys []string) (map[string]catalogdomain.FeatureDef, error)
}

// EntitlementPresenceFinder returns the organization's effective entitlement
// row for a key, or pgx.ErrNoRows when it holds none (satisfied by
// repository.EntitlementRepository). The guard alone cannot tell "no row" from
// "row with enabled=false": both surface as ORGANIZATION_FEATURE_NOT_ENTITLED.
type EntitlementPresenceFinder interface {
	FindEffective(ctx context.Context, organizationID, featureKey string, at time.Time) (model.Entitlement, error)
}

// QuotaEnforcer checks that an organization still has room for one more unit
// of a quota feature, counting usage through the registered QuotaMeter.
type QuotaEnforcer struct {
	guard    *EntitlementGuard
	presence EntitlementPresenceFinder
	features FeatureDefReader
	meters   *QuotaMeterRegistry
	now      func() time.Time
}

func NewQuotaEnforcer(guard *EntitlementGuard, presence EntitlementPresenceFinder, features FeatureDefReader, meters *QuotaMeterRegistry, now func() time.Time) *QuotaEnforcer {
	if now == nil {
		now = time.Now
	}
	return &QuotaEnforcer{guard: guard, presence: presence, features: features, meters: meters, now: now}
}

// RequireRoom returns QUOTA_EXCEEDED when usage already reached the limit,
// FEATURE_NOT_ENABLED when the feature is not entitled, and nil otherwise.
// Platform organizations are bypassed. For the keys in
// uncappedWhenAbsentQuotaKeys an organization with no entitlement row is
// uncapped (nil); a row with enabled=false still yields FEATURE_NOT_ENABLED.
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
	if IsUncappedWhenAbsent(featureKey) {
		absent, err := e.entitlementAbsent(ctx, organizationID, featureKey)
		if err != nil {
			return err
		}
		if absent {
			return nil
		}
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

func (e *QuotaEnforcer) entitlementAbsent(ctx context.Context, organizationID, featureKey string) (bool, error) {
	if e.presence == nil {
		return false, quotaConfigError("entitlement presence finder is not configured")
	}
	_, err := e.presence.FindEffective(ctx, strings.TrimSpace(organizationID), featureKey, e.now().UTC())
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, coreerrors.Wrap("ENTITLEMENT_EVALUATION_FAILED",
			"failed to evaluate organization entitlement", http.StatusInternalServerError, err)
	}
	return false, nil
}

func quotaConfigError(msg string) error {
	return coreerrors.New("QUOTA_ENFORCER_MISCONFIGURED", msg, http.StatusInternalServerError)
}
