package landing

import (
	"zyad.cloud/internal/modules/landing/routes"

	"github.com/gin-gonic/gin"
)

// Module groups Landing Page dependencies and route registration.
type Module struct{}

func NewModule() *Module {
	return &Module{}
}

func (m *Module) RegisterRoutes(router gin.IRoutes) {
	routes.Register(router)
}

func RegisterRoutes(router gin.IRoutes) {
	NewModule().RegisterRoutes(router)
}
