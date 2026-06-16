package app

import (
	"log/slog"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/core/middleware"
	notificationhandler "zyad.cloud/internal/core/notification/handler"
	permissionhandler "zyad.cloud/internal/core/permission/handler"
	organizationhandler "zyad.cloud/internal/modules/organization/handler"
	userhandler "zyad.cloud/internal/modules/user/handler"
	pgdatabase "zyad.cloud/internal/platform/database"
	"zyad.cloud/internal/platform/redis"
)

type Dependencies struct {
	Config                           config.Config
	Logger                           *slog.Logger
	DB                               *pgdatabase.Pool
	Redis                            *redis.Client
	NotificationHandler              *notificationhandler.NotificationHandler
	NotificationLogHandler           *notificationhandler.LogHandler
	NotificationPreferenceHandler    *notificationhandler.PreferenceHandler
	NotificationTemplateHandler      *notificationhandler.TemplateHandler
	NotificationVariableHandler      *notificationhandler.VariableHandler
	PermissionHandler                *permissionhandler.Handler
	OrganizationDomainHandler        *organizationhandler.DomainHandler
	OrganizationEntitlementHandler   *organizationhandler.EntitlementHandler
	OrganizationImpersonationHandler *organizationhandler.ImpersonationHandler
	OrganizationPlatformHandler      *organizationhandler.PlatformHandler
	OrganizationSelfHandler          *organizationhandler.SelfHandler
	OrganizationSwitchHandler        *organizationhandler.SwitchHandler
	UserAuthHandler                  *userhandler.AuthHandler
	UserHandler                      *userhandler.UserHandler
	Authenticator                    middleware.AccessTokenAuthenticator
	OrganizationResolver             middleware.AuthenticatedOrganizationResolver
	PublicHostResolver               middleware.PublicHostResolver
}
