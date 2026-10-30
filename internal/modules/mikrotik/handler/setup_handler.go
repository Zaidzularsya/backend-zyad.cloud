package handler

import (
	"context"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/dto"
	"zyad.cloud/internal/modules/mikrotik/service"
)

// SetupService is the subset of service.SetupService used by the handler.
type SetupService interface {
	Preview(ctx context.Context, scope coretenant.Scope, routerID string, in service.SetupInput) (domain.SetupPlan, error)
	Apply(ctx context.Context, scope coretenant.Scope, routerID string, in service.SetupInput, planHash, actor string) (domain.Router, error)
	Cleanup(ctx context.Context, scope coretenant.Scope, routerID, actor string) (domain.Router, error)
}

type SetupHandler struct {
	svc SetupService
}

func NewSetupHandler(svc SetupService) *SetupHandler { return &SetupHandler{svc: svc} }

// RegisterRoutes mounts the setup routes under /routers/:id/setup. All of them
// require mikrotik_router.setup.
func (h *SetupHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/routers/:id/setup", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_router.setup"))
	group.POST("/preview", h.Preview)
	group.POST("/apply", h.Apply)
	group.POST("/cleanup", h.Cleanup)
}

func setupInput(r dto.SetupRequest) service.SetupInput {
	return service.SetupInput{
		ProfileName: r.ProfileName, PoolCIDR: r.PoolCIDR, RateLimit: r.RateLimit,
		PlatformHost: r.PlatformHost, TenantIsolirHost: r.TenantIsolirHost, DokuHosts: r.DokuHosts,
		RedirectURL: r.RedirectURL,
	}
}

func (h *SetupHandler) Preview(c *gin.Context) {
	extendWriteDeadline(c, previewWriteDeadline)
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var req dto.SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, err)
		return
	}
	plan, err := h.svc.Preview(c.Request.Context(), scope, c.Param("id"), setupInput(req))
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.SetupPlanFromDomain(plan))
}

func (h *SetupHandler) Apply(c *gin.Context) {
	extendWriteDeadline(c, setupWriteDeadline)
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var req dto.ApplySetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, err)
		return
	}
	router, err := h.svc.Apply(c.Request.Context(), scope, c.Param("id"), setupInput(req.SetupRequest), req.PlanHash, permissionmiddleware.UserID(c))
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.RouterFromDomain(router))
}

func (h *SetupHandler) Cleanup(c *gin.Context) {
	extendWriteDeadline(c, setupWriteDeadline)
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	router, err := h.svc.Cleanup(c.Request.Context(), scope, c.Param("id"), permissionmiddleware.UserID(c))
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.RouterFromDomain(router))
}
