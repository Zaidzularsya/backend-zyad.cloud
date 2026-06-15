package app

import (
	"zyad.cloud/internal/modules/account"
	"zyad.cloud/internal/modules/asset"
	"zyad.cloud/internal/modules/billing"
	"zyad.cloud/internal/modules/contract"
	"zyad.cloud/internal/modules/dashboard"
	"zyad.cloud/internal/modules/landing"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/newsaggregator"
	"zyad.cloud/internal/modules/order"
	"zyad.cloud/internal/modules/organization"
	"zyad.cloud/internal/modules/payment"
	"zyad.cloud/internal/modules/product"
	"zyad.cloud/internal/modules/provisioning"
	"zyad.cloud/internal/modules/radius"
	"zyad.cloud/internal/modules/resource"
	"zyad.cloud/internal/modules/user"

	"github.com/gin-gonic/gin"
)

func registerModuleRoutes(router gin.IRoutes) {
	account.RegisterRoutes(router)
	asset.RegisterRoutes(router)
	billing.RegisterRoutes(router)
	contract.RegisterRoutes(router)
	dashboard.RegisterRoutes(router)
	landing.RegisterRoutes(router)
	mikrotik.RegisterRoutes(router)
	newsaggregator.RegisterRoutes(router)
	order.RegisterRoutes(router)
	organization.RegisterRoutes(router)
	payment.RegisterRoutes(router)
	product.RegisterRoutes(router)
	provisioning.RegisterRoutes(router)
	radius.RegisterRoutes(router)
	resource.RegisterRoutes(router)
	user.RegisterRoutes(router)
}
