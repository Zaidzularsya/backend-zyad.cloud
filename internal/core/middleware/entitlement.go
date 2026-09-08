package middleware

import (
	"context"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"

	"github.com/gin-gonic/gin"
)

// EntitlementChecker is a narrow interface over subscription entitlement
// evaluation. It is defined here (rather than importing the subscription
// module's guard service directly) so internal/core does not depend on
// internal/modules — callers in internal/app adapt the concrete guard
// service to this interface when wiring routes.
type EntitlementChecker interface {
	RequireFeature(ctx context.Context, organizationID string, featureKey string) error
}

// RequireEntitlement gates an entire route group behind a boolean/feature
// entitlement key (e.g. "crm.enabled"). It must run after tenant resolution
// middleware (RequireTenantContext) has populated the tenant context.
func RequireEntitlement(checker EntitlementChecker, featureKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantContext, err := RequireTenantContext(c)
		if err != nil {
			corehttp.Fail(c, err)
			c.Abort()
			return
		}

		if checker == nil {
			corehttp.Fail(c, coreerrors.New(
				"ENTITLEMENT_CHECKER_REQUIRED",
				"entitlement checker is required",
				http.StatusInternalServerError,
			))
			c.Abort()
			return
		}

		if err := checker.RequireFeature(c.Request.Context(), tenantContext.OrganizationID(), featureKey); err != nil {
			corehttp.Fail(c, err)
			c.Abort()
			return
		}

		c.Next()
	}
}
