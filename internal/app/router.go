package app

import (
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
	if deps.PermissionHandler != nil {
		deps.PermissionHandler.RegisterRoutes(protected)
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
	registerModuleRoutes(publicModules)
	return router, nil
}
