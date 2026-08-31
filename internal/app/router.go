package app

import (
	"net/http"
	"strings"
	"time"

	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"

	"github.com/gin-gonic/gin"
)

func newRouter(deps Dependencies) (*gin.Engine, error) {
	if deps.Config.App.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	if err := router.SetTrustedProxies(deps.Config.MultiTenant.TrustedProxyCIDRs); err != nil {
		return nil, err
	}
	router.Use(
		middleware.CORS(),
		middleware.RequestID(),
		middleware.Recovery(deps.Logger),
		gin.Logger(),
	)

	router.GET("/healthz", func(c *gin.Context) {
		payload := gin.H{
			"status":    "ok",
			"service":   deps.Config.App.Name,
			"version":   deps.Config.App.Version,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}

		if deps.DB != nil {
			if err := deps.DB.Ping(c.Request.Context()); err == nil {
				payload["database"] = "up"
			} else {
				payload["database"] = "down"
			}
		}

		if deps.Redis != nil {
			if err := deps.Redis.Ping(c.Request.Context()).Err(); err == nil {
				payload["redis"] = "up"
			} else {
				payload["redis"] = "down"
			}
		}

		corehttp.OK(c, "healthy", payload)
	})

	router.GET("/readyz", func(c *gin.Context) {
		corehttp.OK(c, "ready", gin.H{"ready": true})
	})

	// Media publik (logo/gambar landing) dilayani tanpa auth; provider hanya
	// me-resolve object berkelas public dan menolak path traversal.
	if deps.MediaStorage != nil {
		router.GET("/public/media/*objectKey", func(c *gin.Context) {
			objectKey := strings.TrimPrefix(c.Param("objectKey"), "/")
			filePath, err := deps.MediaStorage.ResolvePublic(objectKey)
			if err != nil {
				c.Status(http.StatusNotFound)
				return
			}
			c.Header("Cache-Control", "public, max-age=3600")
			c.File(filePath)
		})
	}

	api := router.Group("/api/v1")
	api.GET("/meta", func(c *gin.Context) {
		corehttp.OK(c, "meta", gin.H{
			"name":    deps.Config.App.Name,
			"version": deps.Config.App.Version,
			"env":     deps.Config.App.Env,
		})
	})

	if deps.UserAuthHandler != nil {
		deps.UserAuthHandler.RegisterRoutes(api)
	}
	if deps.PublicProductHandler != nil {
		deps.PublicProductHandler.RegisterRoutes(api)
	}
	// DOKU payment notifications authenticate via HMAC signature headers, so
	// the route lives outside both the JWT-protected and tenant-resolved
	// groups.
	if deps.DokuWebhookHandler != nil {
		deps.DokuWebhookHandler.RegisterRoutes(api)
	}

	protected := api.Group("")
	protected.Use(
		middleware.Authenticate(deps.Authenticator),
		middleware.ResolveAuthenticatedOrganization(deps.OrganizationResolver),
	)
	if deps.UserAuthHandler != nil {
		deps.UserAuthHandler.RegisterProtectedRoutes(protected)
	}
	if deps.UserHandler != nil {
		deps.UserHandler.RegisterRoutes(protected)
	}
	if deps.OrganizationSwitchHandler != nil {
		deps.OrganizationSwitchHandler.RegisterRoutes(protected)
	}
	if deps.OrganizationOnboardingHandler != nil {
		deps.OrganizationOnboardingHandler.RegisterRoutes(protected)
	}
	if deps.OrganizationPlatformHandler != nil {
		deps.OrganizationPlatformHandler.RegisterRoutes(protected)
	}
	if deps.OrganizationDomainHandler != nil {
		deps.OrganizationDomainHandler.RegisterRoutes(protected)
	}
	if deps.OrganizationEntitlementHandler != nil {
		deps.OrganizationEntitlementHandler.RegisterRoutes(protected)
	}
	if deps.OrganizationImpersonationHandler != nil {
		deps.OrganizationImpersonationHandler.RegisterRoutes(protected)
	}
	if deps.OrganizationSelfHandler != nil {
		deps.OrganizationSelfHandler.RegisterRoutes(protected)
	}
	if deps.PermissionHandler != nil {
		deps.PermissionHandler.RegisterRoutes(protected)
	}
	if deps.PlatformProductHandler != nil {
		deps.PlatformProductHandler.RegisterRoutes(protected)
	}
	if deps.PlatformSubscriptionHandler != nil {
		deps.PlatformSubscriptionHandler.RegisterRoutes(protected)
	}
	if deps.PlatformBillingHandler != nil {
		deps.PlatformBillingHandler.RegisterRoutes(protected)
	}
	if deps.TenantBillingHandler != nil {
		deps.TenantBillingHandler.RegisterRoutes(protected)
	}
	if deps.NotificationTemplateHandler != nil {
		deps.NotificationTemplateHandler.RegisterRoutes(protected)
	}
	if deps.NotificationVariableHandler != nil {
		deps.NotificationVariableHandler.RegisterRoutes(protected)
	}
	if deps.NotificationLogHandler != nil {
		deps.NotificationLogHandler.RegisterRoutes(protected)
	}
	if deps.NotificationPreferenceHandler != nil {
		deps.NotificationPreferenceHandler.RegisterRoutes(protected)
	}
	if deps.NotificationHandler != nil {
		deps.NotificationHandler.RegisterInternalRoutes(protected)
	}
	// Landing Page API (Admin)
	// Routes are registered directly here per-handler rather than through a
	// module.go/routes.go indirection — the landing module intentionally has
	// no such wrapper; don't recreate one.
	if deps.LandingAdminPageHandler != nil {
		deps.LandingAdminPageHandler.RegisterRoutes(protected, deps.PermissionChecker)
	}
	if deps.LandingAdminSectionHandler != nil {
		deps.LandingAdminSectionHandler.RegisterRoutes(protected, deps.PermissionChecker)
	}
	if deps.LandingAdminBrandingHandler != nil {
		deps.LandingAdminBrandingHandler.RegisterRoutes(protected, deps.PermissionChecker)
	}
	if deps.LandingAdminDomainHandler != nil {
		deps.LandingAdminDomainHandler.RegisterRoutes(protected, deps.PermissionChecker)
	}
	if deps.LandingAdminFormHandler != nil {
		deps.LandingAdminFormHandler.RegisterRoutes(protected, deps.PermissionChecker)
	}
	if deps.LandingAdminSubmissionHandler != nil {
		deps.LandingAdminSubmissionHandler.RegisterRoutes(protected, deps.PermissionChecker)
	}
	if deps.LandingAdminCTAHandler != nil {
		deps.LandingAdminCTAHandler.RegisterRoutes(protected, deps.PermissionChecker)
	}
	if deps.LandingAdminTemplateHandler != nil {
		deps.LandingAdminTemplateHandler.RegisterRoutes(protected, deps.PermissionChecker)
	}
	if deps.LandingAdminMediaHandler != nil {
		deps.LandingAdminMediaHandler.RegisterRoutes(protected, deps.PermissionChecker)
	}
	if deps.LandingAdminNavigationHandler != nil {
		deps.LandingAdminNavigationHandler.RegisterRoutes(protected, deps.PermissionChecker)
	}
	if deps.LandingAdminRevisionHandler != nil {
		deps.LandingAdminRevisionHandler.RegisterRoutes(protected, deps.PermissionChecker)
	}
	if deps.LandingAdminScheduleHandler != nil {
		deps.LandingAdminScheduleHandler.RegisterRoutes(protected, deps.PermissionChecker)
	}
	if deps.LandingAdminIntegrationHandler != nil {
		deps.LandingAdminIntegrationHandler.RegisterRoutes(protected, deps.PermissionChecker)
	}

	publicTenantMiddleware, err := middleware.ResolvePublicOrganization(
		deps.PublicHostResolver,
		middleware.PublicHostOptions{
			TrustForwardedHost: deps.Config.MultiTenant.TrustForwardedHost,
			TrustedProxyCIDRs:  deps.Config.MultiTenant.TrustedProxyCIDRs,
		},
	)
	if err != nil {
		return nil, err
	}
	publicModules := api.Group("")
	publicModules.Use(
		publicTenantMiddleware,
		middleware.RequireActiveTenant(),
	)
	if deps.PublicLandingHandler != nil {
		deps.PublicLandingHandler.RegisterRoutes(publicModules)
	}
	registerModuleRoutes(publicModules)
	return router, nil
}
