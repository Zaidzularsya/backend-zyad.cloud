package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/dto"
	"zyad.cloud/internal/modules/crm/service"
)

type LeadDashboardHandler struct {
	svc service.LeadDashboardService
}

func NewLeadDashboardHandler(svc service.LeadDashboardService) *LeadDashboardHandler {
	return &LeadDashboardHandler{svc: svc}
}

// RegisterRoutes adds GET /leads/dashboard. Gin matches the static segment
// before /leads/:id, so registration order does not matter.
func (h *LeadDashboardHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	router.GET("/leads/dashboard", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.read"), h.Get)
}

func (h *LeadDashboardHandler) Get(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var query dto.LeadDashboardQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	dashboard, rng, err := h.svc.Dashboard(c.Request.Context(), scope, service.LeadDashboardQuery{
		From:        query.From,
		To:          query.To,
		Granularity: query.Granularity,
	})
	if err != nil {
		if service.IsLeadDashboardValidationError(err) {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
			return
		}
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "success", dto.LeadDashboardFromDomain(dashboard, rng))
}
