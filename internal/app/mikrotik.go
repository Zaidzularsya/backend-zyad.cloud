package app

import (
	"time"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	mikrotikhandler "zyad.cloud/internal/modules/mikrotik/handler"
	mikrotikrepo "zyad.cloud/internal/modules/mikrotik/repository"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
	"zyad.cloud/internal/platform/database"
)

// buildMikrotik assembles the Mikrotik router module: repository, production
// RouterOS client factory, services and HTTP handlers.
func buildMikrotik(cfg config.Config, db *database.Pool) (*mikrotikhandler.RouterHandler, *mikrotikhandler.SetupHandler) {
	repo := mikrotikrepo.NewRouterRepository(db)
	svc := mikrotikservice.NewRouterService(
		repo,
		mikrotikservice.NewPlatformClientFactory(),
		cfg.App.Secret,
		cfg.Mikrotik.AllowedPrivateCIDRs,
	)
	setup := mikrotikservice.NewSetupService(svc, repo, mikrotikrepo.NewAdvisoryLocker(db),
		mikrotikservice.SetupOptions{AllowREST: cfg.Mikrotik.SetupAllowREST})
	return mikrotikhandler.NewRouterHandler(svc), mikrotikhandler.NewSetupHandler(setup)
}

// Every call that logs in to a tenant router (create, update, check, setup/*)
// is limited per organization: each one is a login attempt against a
// tenant-chosen host (credential oracle, the router may blacklist the platform
// IP) and can hold a socket for many seconds.
const (
	mikrotikProbeLimit       = 20
	mikrotikProbeLimitWindow = time.Minute
)

// registerMikrotikRoutes mounts /app/mikrotik under the authenticated group.
// Tenant rules match CRM/receivable and the whole group is gated by the
// mikrotik.enabled entitlement. Order per route: tenant + entitlement guards,
// then the per-organization rate limit (router-contacting routes only), then
// the permission check. Kept separate from newRouter so the gates can be tested
// without the full dependency graph.
func registerMikrotikRoutes(protected *gin.RouterGroup, deps Dependencies) {
	if deps.MikrotikRouterHandler == nil {
		return
	}
	group := protected.Group("/app/mikrotik")
	group.Use(
		middleware.RequireActiveTenant(),
		middleware.RequireCustomerOrPlatformTenant(),
		middleware.RequireEntitlement(deps.CRMEntitlementChecker, "mikrotik.enabled"),
	)
	// Fail open when no counter is configured or Redis errors (middleware behavior).
	probeLimit := middleware.RateLimit(deps.MikrotikRateCounter, "rl:mikrotik:org:", mikrotikProbeLimit, mikrotikProbeLimitWindow,
		func(c *gin.Context) string {
			if tc, ok := coretenant.FromContext(c.Request.Context()); ok {
				return tc.OrganizationID()
			}
			return "unknown"
		})
	deps.MikrotikRouterHandler.RegisterRoutes(group, deps.PermissionChecker, probeLimit)
	if deps.MikrotikSetupHandler != nil {
		deps.MikrotikSetupHandler.RegisterRoutes(group, deps.PermissionChecker, probeLimit)
	}
}
