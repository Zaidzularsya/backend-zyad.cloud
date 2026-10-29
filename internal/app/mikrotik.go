package app

import (
	"zyad.cloud/internal/config"
	mikrotikhandler "zyad.cloud/internal/modules/mikrotik/handler"
	mikrotikrepo "zyad.cloud/internal/modules/mikrotik/repository"
	mikrotikservice "zyad.cloud/internal/modules/mikrotik/service"
	"zyad.cloud/internal/platform/database"
)

// buildMikrotik assembles the Mikrotik router module: repository, production
// RouterOS client factory, service and HTTP handler.
func buildMikrotik(cfg config.Config, db *database.Pool) *mikrotikhandler.RouterHandler {
	svc := mikrotikservice.NewRouterService(
		mikrotikrepo.NewRouterRepository(db),
		mikrotikservice.NewPlatformClientFactory(),
		cfg.App.Secret,
		cfg.Mikrotik.AllowedPrivateCIDRs,
	)
	return mikrotikhandler.NewRouterHandler(svc)
}
