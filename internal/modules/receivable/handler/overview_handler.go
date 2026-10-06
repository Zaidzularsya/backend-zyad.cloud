package handler

import (
	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/receivable/dto"
	"zyad.cloud/internal/modules/receivable/service"
)

type OverviewHandler struct{ svc service.OverviewService }

func NewOverviewHandler(svc service.OverviewService) *OverviewHandler {
	return &OverviewHandler{svc: svc}
}

func (h *OverviewHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	router.GET("/overview", permissionmiddleware.RequireOrganizationOrGlobal(p, "contract.read"), h.Get)
}

func (h *OverviewHandler) Get(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	o, err := h.svc.Get(c.Request.Context(), scope)
	if err != nil {
		failReceivable(c, err)
		return
	}
	corehttp.OK(c, "success", dto.OverviewFromDomain(o))
}
