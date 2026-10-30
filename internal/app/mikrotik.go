package app

import (
	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/core/middleware"
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
	setup := mikrotikservice.NewSetupService(svc, repo)
	return mikrotikhandler.NewRouterHandler(svc), mikrotikhandler.NewSetupHandler(setup)
}

// registerMikrotikRoutes mounts /app/mikrotik under the authenticated group.
// Tenant rules match CRM/receivable and the whole group is gated by the
// mikrotik.enabled entitlement. Kept separate from newRouter so the gate can be
// tested without the full dependency graph.
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
	deps.MikrotikRouterHandler.RegisterRoutes(group, deps.PermissionChecker)
	if deps.MikrotikSetupHandler != nil {
		deps.MikrotikSetupHandler.RegisterRoutes(group, deps.PermissionChecker)
	}
}
