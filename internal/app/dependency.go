package app

import (
	"log/slog"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/core/middleware"
	notificationhandler "zyad.cloud/internal/core/notification/handler"
	permissionhandler "zyad.cloud/internal/core/permission/handler"
	userhandler "zyad.cloud/internal/modules/user/handler"
	pgdatabase "zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/redis"
)

type Dependencies struct {
	Config                        config.Config
	Logger                        *slog.Logger
	DB                            *pgdatabase.Pool
	Redis                         *redis.Client
	NotificationHandler           *notificationhandler.NotificationHandler
	NotificationLogHandler        *notificationhandler.LogHandler
	NotificationPreferenceHandler *notificationhandler.PreferenceHandler
	NotificationTemplateHandler   *notificationhandler.TemplateHandler
	NotificationVariableHandler   *notificationhandler.VariableHandler
	PermissionHandler             *permissionhandler.Handler
	UserAuthHandler               *userhandler.AuthHandler
	UserHandler                   *userhandler.UserHandler
	Authenticator                 middleware.AccessTokenAuthenticator
}
