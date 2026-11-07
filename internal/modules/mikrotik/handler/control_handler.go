package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/dto"
	"zyad.cloud/internal/modules/mikrotik/service"
	"zyad.cloud/internal/shared/response"
)

// ControlService is the subset of service.ControlService used by the handler.
type ControlService interface {
	State(ctx context.Context, scope coretenant.Scope) (service.ControlState, error)
	SetGraceDays(ctx context.Context, scope coretenant.Scope, days int, actor string) (service.ControlState, error)
	SetMode(ctx context.Context, scope coretenant.Scope, in service.SetModeInput, actor string) (service.ControlState, error)
	ListPilot(ctx context.Context, scope coretenant.Scope) ([]service.PilotView, error)
	AddPilot(ctx context.Context, scope coretenant.Scope, accountID, actor string) error
	RemovePilot(ctx context.Context, scope coretenant.Scope, accountID, actor string) error
	StartPilotTest(ctx context.Context, scope coretenant.Scope, accountID, actor string) (time.Time, error)
	StopPilotTest(ctx context.Context, scope coretenant.Scope, accountID, actor string) error
	RestoreAll(ctx context.Context, scope coretenant.Scope, actor string) (service.RestoreAllResult, error)
}

type ControlHandler struct {
	svc ControlService
}

func NewControlHandler(svc ControlService) *ControlHandler { return &ControlHandler{svc: svc} }

// RegisterRoutes mounts the control center routes. None of them contacts a
// router directly (tasks do), so probeLimit is not applied.
func (h *ControlHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker, _ gin.HandlerFunc) {
	read := permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_control.read")
	update := permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_control.update")
	group := router.Group("/control")
	group.GET("", read, h.State)
	group.PUT("/grace-days", update, h.SetGraceDays)
	group.PUT("/mode", update, h.SetMode)
	group.GET("/pilot", read, h.ListPilot)
	group.PUT("/pilot/:account_id", update, h.AddPilot)
	group.DELETE("/pilot/:account_id", update, h.RemovePilot)
	group.POST("/pilot/:account_id/test", update, h.StartPilotTest)
	group.DELETE("/pilot/:account_id/test", update, h.StopPilotTest)
	group.POST("/restore-all", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_control.restore"), h.RestoreAll)
}

func (h *ControlHandler) State(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	st, err := h.svc.State(c.Request.Context(), scope)
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.ControlStateFromService(st))
}

func (h *ControlHandler) SetGraceDays(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var req dto.SetGraceDaysRequest
	if err := bindStrictJSON(c, &req); err != nil {
		validationFail(c, err)
		return
	}
	st, err := h.svc.SetGraceDays(c.Request.Context(), scope, *req.GraceDays, permissionmiddleware.UserID(c))
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.ControlStateFromService(st))
}

func (h *ControlHandler) SetMode(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var req dto.SetModeRequest
	if err := bindStrictJSON(c, &req); err != nil {
		validationFail(c, err)
		return
	}
	in, err := req.ToInput()
	if err != nil {
		validationFail(c, err)
		return
	}
	if in.Exemptions != nil {
		// A malformed id would otherwise reach PostgreSQL as a failing uuid cast.
		for _, id := range in.Exemptions.AccountIDs {
			if !validUUID(id) {
				failRouter(c, mikrotik.ErrAccountNotFound)
				return
			}
		}
	}
	st, err := h.svc.SetMode(c.Request.Context(), scope, in, permissionmiddleware.UserID(c))
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.ControlStateFromService(st))
}

func (h *ControlHandler) ListPilot(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	items, err := h.svc.ListPilot(c.Request.Context(), scope)
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.PilotListFromService(items))
}

// pilotAccount returns the scope and a well-formed account_id path param.
func pilotAccount(c *gin.Context) (coretenant.Scope, string, bool) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return coretenant.Scope{}, "", false
	}
	id := c.Param("account_id")
	if !validUUID(id) {
		failRouter(c, mikrotik.ErrAccountNotFound)
		return coretenant.Scope{}, "", false
	}
	return scope, id, true
}

func (h *ControlHandler) AddPilot(c *gin.Context) {
	scope, id, ok := pilotAccount(c)
	if !ok {
		return
	}
	if err := h.svc.AddPilot(c.Request.Context(), scope, id, permissionmiddleware.UserID(c)); err != nil {
		failRouter(c, err)
		return
	}
	corehttp.NoContent(c)
}

func (h *ControlHandler) RemovePilot(c *gin.Context) {
	scope, id, ok := pilotAccount(c)
	if !ok {
		return
	}
	if err := h.svc.RemovePilot(c.Request.Context(), scope, id, permissionmiddleware.UserID(c)); err != nil {
		failRouter(c, err)
		return
	}
	corehttp.NoContent(c)
}

func (h *ControlHandler) StartPilotTest(c *gin.Context) {
	scope, id, ok := pilotAccount(c)
	if !ok {
		return
	}
	until, err := h.svc.StartPilotTest(c.Request.Context(), scope, id, permissionmiddleware.UserID(c))
	if err != nil {
		failRouter(c, err)
		return
	}
	response.JSON(c, http.StatusAccepted, "accepted", dto.PilotTestResponse{TestUntil: until}, nil)
}

func (h *ControlHandler) StopPilotTest(c *gin.Context) {
	scope, id, ok := pilotAccount(c)
	if !ok {
		return
	}
	if err := h.svc.StopPilotTest(c.Request.Context(), scope, id, permissionmiddleware.UserID(c)); err != nil {
		failRouter(c, err)
		return
	}
	corehttp.NoContent(c)
}

func (h *ControlHandler) RestoreAll(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	res, err := h.svc.RestoreAll(c.Request.Context(), scope, permissionmiddleware.UserID(c))
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.RestoreAllFromService(res))
}
