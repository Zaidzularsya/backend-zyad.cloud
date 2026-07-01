package app

import (
	"log/slog"

	"zyad.cloud/internal/config"
	"zyad.cloud/internal/core/middleware"
	notificationhandler "zyad.cloud/internal/core/notification/handler"
	permissionhandler "zyad.cloud/internal/core/permission/handler"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	billinghandler "zyad.cloud/internal/modules/billing/handler"
	landinghandler "zyad.cloud/internal/modules/landing/handler"
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
	PlatformBillingHandler           *billinghandler.PlatformBillingHandler
	TenantBillingHandler             *billinghandler.TenantBillingHandler
	OrganizationDomainHandler        *organizationhandler.DomainHandler
	OrganizationEntitlementHandler   *organizationhandler.EntitlementHandler
	OrganizationImpersonationHandler *organizationhandler.ImpersonationHandler
	OrganizationOnboardingHandler    *organizationhandler.OnboardingHandler
	OrganizationPlatformHandler      *organizationhandler.PlatformHandler
	OrganizationSelfHandler          *organizationhandler.SelfHandler
	OrganizationSwitchHandler        *organizationhandler.SwitchHandler
	UserAuthHandler                  *userhandler.AuthHandler
	UserHandler                      *userhandler.UserHandler
	LandingAdminPageHandler          *landinghandler.AdminPageHandler
	LandingAdminSectionHandler       *landinghandler.AdminSectionHandler
	LandingAdminBrandingHandler      *landinghandler.AdminBrandingHandler
	LandingAdminDomainHandler        *landinghandler.AdminDomainHandler
	LandingAdminFormHandler          *landinghandler.AdminFormHandler
	LandingAdminSubmissionHandler    *landinghandler.AdminSubmissionHandler
	PublicLandingHandler             *landinghandler.PublicLandingHandler
	LandingAdminCTAHandler           *landinghandler.AdminCTAHandler
	LandingAdminTemplateHandler      *landinghandler.AdminTemplateHandler
	LandingAdminMediaHandler         *landinghandler.AdminMediaHandler
	LandingAdminNavigationHandler    *landinghandler.AdminNavigationHandler
	LandingAdminRevisionHandler      *landinghandler.AdminRevisionHandler
	LandingAdminScheduleHandler      *landinghandler.AdminScheduleHandler
	LandingAdminIntegrationHandler   *landinghandler.AdminIntegrationHandler
	PermissionChecker                permissionmiddleware.PermissionChecker
	Authenticator                    middleware.AccessTokenAuthenticator
	OrganizationResolver             middleware.AuthenticatedOrganizationResolver
	PublicHostResolver               middleware.PublicHostResolver
}
