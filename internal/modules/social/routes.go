package social

import (
	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/modules/social/handler"
)

// Handlers are the HTTP entry points of the social module; nil ones are skipped.
type Handlers struct {
	Public *handler.PublicHandler
	Me     *handler.MeHandler
	// Tenant is mounted by the router on /app/social with tenant middleware.
	Tenant *handler.TenantHandler
}

// RegisterRoutes mounts public routes on the plain api group and /social/me/* on
// the authenticated group (no tenant middleware: data is keyed by user).
func RegisterRoutes(api, protected *gin.RouterGroup, h Handlers) {
	if h.Public != nil {
		h.Public.RegisterRoutes(api)
	}
	if h.Me != nil {
		h.Me.RegisterRoutes(protected)
	}
}
