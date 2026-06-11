package app

import (
	"log/slog"

	"zyad.cloud/internal/config"
	permissionhandler "zyad.cloud/internal/core/permission/handler"
	userhandler "zyad.cloud/internal/modules/user/handler"
	pgdatabase "zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/redis"
)

type Dependencies struct {
	Config            config.Config
	Logger            *slog.Logger
	DB                *pgdatabase.Pool
	Redis             *redis.Client
	PermissionHandler *permissionhandler.Handler
	UserAuthHandler   *userhandler.AuthHandler
}
