package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/whatsapp/domain"
	"zyad.cloud/internal/modules/whatsapp/dto"
	"zyad.cloud/internal/modules/whatsapp/service"
)

type SessionHandler struct {
	svc *service.SessionService
}

func NewSessionHandler(svc *service.SessionService) *SessionHandler {
	return &SessionHandler{svc: svc}
}

// RegisterRoutes mounts /sessions under the /app/whatsapp group (tenant and
// whatsapp.enabled checks are applied by the group in internal/app/router.go).
// Sessions are always addressed by id; the WAHA session name is never
// accepted from or returned to clients.
func (h *SessionHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	read := permissionmiddleware.RequireOrganizationOrGlobal(p, domain.PermissionSessionRead)
	manage := permissionmiddleware.RequireOrganizationOrGlobal(p, domain.PermissionSessionManage)

	group := router.Group("/sessions")
	group.GET("", read, h.List)
	group.POST("", manage, h.Create)
	group.GET("/:id", read, h.Get)
	group.PATCH("/:id", manage, h.Update)
	group.DELETE("/:id", manage, h.Delete)
	group.GET("/:id/status", read, h.Status)
	group.POST("/:id/start", manage, h.Start)
	group.POST("/:id/stop", manage, h.Stop)
	group.POST("/:id/logout", manage, h.Logout)
	group.GET("/:id/qr", manage, h.QR)
	group.POST("/:id/pairing-code", manage, h.PairingCode)
}

func requireScope(c *gin.Context) (coretenant.Scope, bool) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return coretenant.Scope{}, false
	}
	return scope, true
}

func failValidation(c *gin.Context, err error) {
	corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
}

func (h *SessionHandler) List(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	sessions, err := h.svc.List(c.Request.Context(), scope)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.SessionListFromDomain(sessions))
}

func (h *SessionHandler) Create(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var req dto.CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failValidation(c, err)
		return
	}
	session, err := h.svc.Create(c.Request.Context(), scope, service.CreateSessionInput{
		DisplayName:    req.DisplayName,
		Purpose:        domain.SessionPurpose(req.Purpose),
		IsDefault:      req.IsDefault,
		AutoCreateLead: req.AutoCreateLead,
		ActorUserID:    permissionmiddleware.UserID(c),
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "created", dto.SessionFromDomain(session))
}

func (h *SessionHandler) Get(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	session, err := h.svc.Get(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.SessionFromDomain(session))
}

func (h *SessionHandler) Update(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var req dto.UpdateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failValidation(c, err)
		return
	}
	input := service.UpdateSessionInput{
		DisplayName:    req.DisplayName,
		IsDefault:      req.IsDefault,
		AutoCreateLead: req.AutoCreateLead,
		ActorUserID:    permissionmiddleware.UserID(c),
	}
	if req.Purpose != nil {
		purpose := domain.SessionPurpose(*req.Purpose)
		input.Purpose = &purpose
	}
	session, err := h.svc.Update(c.Request.Context(), scope, c.Param("id"), input)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "updated", dto.SessionFromDomain(session))
}

func (h *SessionHandler) Delete(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), scope, c.Param("id"), permissionmiddleware.UserID(c)); err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "deleted", nil)
}

func (h *SessionHandler) Status(c *gin.Context) {
	h.respondSession(c, h.svc.Status)
}

func (h *SessionHandler) Start(c *gin.Context) {
	h.respondSession(c, h.svc.Start)
}

func (h *SessionHandler) Stop(c *gin.Context) {
	h.respondSession(c, h.svc.Stop)
}

func (h *SessionHandler) Logout(c *gin.Context) {
	h.respondSession(c, h.svc.Logout)
}

func (h *SessionHandler) respondSession(
	c *gin.Context,
	action func(ctx context.Context, scope coretenant.Scope, id string) (domain.Session, error),
) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	session, err := action(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.SessionFromDomain(session))
}

func (h *SessionHandler) QR(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	qr, err := h.svc.QR(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.SessionQRResponse{QR: qr.DataURL, Status: string(qr.Status)})
}

func (h *SessionHandler) PairingCode(c *gin.Context) {
	scope, ok := requireScope(c)
	if !ok {
		return
	}
	var req dto.PairingCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		failValidation(c, err)
		return
	}
	code, err := h.svc.PairingCode(c.Request.Context(), scope, c.Param("id"), req.Phone)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "success", dto.PairingCodeResponse{Code: code})
}
