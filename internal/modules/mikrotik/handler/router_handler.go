package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/dto"
	"zyad.cloud/internal/modules/mikrotik/service"
	"zyad.cloud/internal/shared/response"
)

// RouterService is the subset of service.RouterService used by the handler.
type RouterService interface {
	Register(ctx context.Context, scope coretenant.Scope, in service.RegisterInput) (domain.Router, error)
	Get(ctx context.Context, scope coretenant.Scope, id string) (domain.Router, error)
	List(ctx context.Context, scope coretenant.Scope, limit, offset int) ([]domain.Router, int64, error)
	Check(ctx context.Context, scope coretenant.Scope, id string) (domain.Router, error)
	Update(ctx context.Context, scope coretenant.Scope, id string, in service.UpdateInput) (domain.Router, error)
	Delete(ctx context.Context, scope coretenant.Scope, id, by string) error
}

type RouterHandler struct {
	svc RouterService
}

func NewRouterHandler(svc RouterService) *RouterHandler {
	return &RouterHandler{svc: svc}
}

// RegisterRoutes registers router routes under the given parent group, which
// carries the tenant and mikrotik.enabled entitlement guards. probeLimit (may
// be nil) runs before the permission check on every route that logs in to the
// tenant's router (create, update, check); list/get/delete are not limited.
func (h *RouterHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker, probeLimit gin.HandlerFunc) {
	group := router.Group("/routers")
	limit := orPass(probeLimit)

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_router.read"), h.List)
	group.POST("", limit, permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_router.create"), h.Create)
	group.GET("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_router.read"), h.Get)
	group.PATCH("/:id", limit, permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_router.update"), h.Update)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_router.delete"), h.Delete)
	group.POST("/:id/check", limit, permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_router.update"), h.Check)
}

// orPass returns mw, or a no-op middleware when mw is nil.
func orPass(mw gin.HandlerFunc) gin.HandlerFunc {
	if mw == nil {
		return func(c *gin.Context) { c.Next() }
	}
	return mw
}

func scopeOrFail(c *gin.Context) (coretenant.Scope, bool) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return coretenant.Scope{}, false
	}
	return scope, true
}

func validationFail(c *gin.Context, err error) {
	corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
}

func (h *RouterHandler) List(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var query dto.RouterListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		validationFail(c, err)
		return
	}
	page, perPage := query.Page, query.PerPage
	if page <= 0 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 20
	}
	if perPage > domain.MaxRouterPageSize {
		// Same clamp as the repository, so meta.per_page is the real page size.
		perPage = domain.MaxRouterPageSize
	}
	routers, total, err := h.svc.List(c.Request.Context(), scope, perPage, (page-1)*perPage)
	if err != nil {
		failRouter(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "success", dto.RouterListFromDomain(routers), dto.BuildMeta(page, perPage, total))
}

func (h *RouterHandler) Create(c *gin.Context) {
	extendWriteDeadline(c, probeWriteDeadline)
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var req dto.CreateRouterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, err)
		return
	}
	router, err := h.svc.Register(c.Request.Context(), scope, service.RegisterInput{
		Name: req.Name, Host: req.Host, Port: req.Port, Username: req.Username, Password: req.Password,
		UseTLS: req.UseTLS, TLSFingerprint: req.TLSFingerprint, ActorUserID: permissionmiddleware.UserID(c),
	})
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.Created(c, "success", dto.RouterFromDomain(router))
}

func (h *RouterHandler) Get(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	router, err := h.svc.Get(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.RouterFromDomain(router))
}

func (h *RouterHandler) Update(c *gin.Context) {
	extendWriteDeadline(c, probeWriteDeadline)
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var req dto.UpdateRouterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		validationFail(c, err)
		return
	}
	router, err := h.svc.Update(c.Request.Context(), scope, c.Param("id"), service.UpdateInput{
		Name: req.Name, Host: req.Host, Port: req.Port, Username: req.Username, Password: req.Password,
		UseTLS: req.UseTLS, TLSFingerprint: req.TLSFingerprint, ActorUserID: permissionmiddleware.UserID(c),
	})
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.RouterFromDomain(router))
}

func (h *RouterHandler) Delete(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), scope, c.Param("id"), permissionmiddleware.UserID(c)); err != nil {
		failRouter(c, err)
		return
	}
	corehttp.NoContent(c)
}

// Check probes the router. The service persists the outcome even when the probe
// fails, so a failure here only needs to be reported.
func (h *RouterHandler) Check(c *gin.Context) {
	extendWriteDeadline(c, probeWriteDeadline)
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	router, err := h.svc.Check(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.RouterFromDomain(router))
}

// failRouter maps module errors to HTTP responses. Unknown errors are never
// echoed: they may contain connection details or credentials.
func failRouter(c *gin.Context, err error) {
	var invalidErr *mikrotik.InvalidInputError
	var tlsErr *mikrotik.TLSUntrustedError
	var conflictErr *mikrotik.SetupConflictError
	var stepErr *mikrotik.SetupStepError
	var appErr *coreerrors.AppError
	switch {
	case errors.Is(err, mikrotik.ErrRouterNotFound):
		response.Error(c, http.StatusNotFound, "MIKROTIK_ROUTER_NOT_FOUND", "router not found")
	case errors.Is(err, mikrotik.ErrSecretNotFound):
		response.Error(c, http.StatusNotFound, "MIKROTIK_SECRET_NOT_FOUND", "ppp secret not found")
	case errors.Is(err, mikrotik.ErrAccountNotFound):
		response.Error(c, http.StatusNotFound, "MIKROTIK_ACCOUNT_NOT_FOUND", "customer account not found")
	case errors.Is(err, mikrotik.ErrProductNotFound):
		response.Error(c, http.StatusNotFound, "MIKROTIK_PRODUCT_NOT_FOUND", "product not found")
	case errors.Is(err, mikrotik.ErrSecretAlreadyPaired):
		response.Error(c, http.StatusConflict, "MIKROTIK_SECRET_ALREADY_PAIRED", "ppp secret is already paired to a customer")
	case errors.Is(err, mikrotik.ErrSecretIsolated):
		response.Error(c, http.StatusConflict, "MIKROTIK_SECRET_ISOLATED", "ppp secret is isolated; restore its profile before unpairing")
	case errors.Is(err, mikrotik.ErrSecretHasActiveTask):
		response.Error(c, http.StatusConflict, "MIKROTIK_SECRET_HAS_ACTIVE_TASK",
			"PPP secret tidak dapat dilepas karena masih ada tugas isolir/pemulihan yang menunggu atau sedang berjalan. Coba lagi setelah tugas selesai.")
	case errors.Is(err, mikrotik.ErrRouterHasIsolatedSecrets):
		response.Error(c, http.StatusConflict, "MIKROTIK_ROUTER_HAS_ISOLATED_SECRETS",
			"Router tidak dapat dihapus karena masih ada pelanggan yang terisolir atau tugas isolir/pemulihan yang belum selesai. Pulihkan pelanggan dan tunggu tugas selesai terlebih dahulu.")
	case errors.Is(err, mikrotik.ErrSecretNotPairable):
		response.Error(c, http.StatusConflict, "MIKROTIK_SECRET_NOT_PAIRABLE", err.Error())
	case errors.Is(err, mikrotik.ErrRouterNameTaken):
		response.Error(c, http.StatusConflict, "MIKROTIK_ROUTER_NAME_TAKEN", "router name is already used")
	case errors.As(err, &invalidErr):
		response.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", invalidErr.Message)
	case errors.Is(err, mikrotik.ErrPlanChanged):
		response.Error(c, http.StatusConflict, "MIKROTIK_SETUP_PLAN_CHANGED", "router state changed since preview; preview again before applying")
	case errors.Is(err, mikrotik.ErrSetupRESTUnverified):
		response.Error(c, http.StatusUnprocessableEntity, "MIKROTIK_SETUP_REST_UNVERIFIED", err.Error())
	case errors.Is(err, mikrotik.ErrSetupInProgress):
		response.Error(c, http.StatusConflict, "MIKROTIK_SETUP_IN_PROGRESS", err.Error())
	case errors.As(err, &conflictErr):
		response.Error(c, http.StatusConflict, "MIKROTIK_SETUP_CONFLICT", conflictErr.Error())
	case errors.Is(err, mikrotik.ErrHostNotAllowed):
		response.Error(c, http.StatusUnprocessableEntity, "MIKROTIK_HOST_NOT_ALLOWED", "router host is not allowed")
	case errors.Is(err, mikrotik.ErrTLSRequired):
		response.Error(c, http.StatusUnprocessableEntity, "MIKROTIK_TLS_REQUIRED", "a publicly reachable router must use TLS")
	case errors.As(err, &tlsErr):
		response.ErrorWithData(c, http.StatusUnprocessableEntity, "MIKROTIK_TLS_UNTRUSTED",
			"router tls certificate is not trusted",
			gin.H{"fingerprint": tlsErr.Fingerprint, "pin_mismatch": tlsErr.PinMismatch})
	case errors.Is(err, mikrotik.ErrTLSUntrusted):
		response.Error(c, http.StatusUnprocessableEntity, "MIKROTIK_TLS_UNTRUSTED", "router tls certificate is not trusted")
	case errors.Is(err, mikrotik.ErrAuthFailed):
		response.Error(c, http.StatusBadGateway, "MIKROTIK_AUTH_FAILED", "router rejected the credentials")
	case errors.Is(err, mikrotik.ErrUnreachable):
		response.Error(c, http.StatusBadGateway, "MIKROTIK_UNREACHABLE", "router is unreachable")
	case errors.Is(err, mikrotik.ErrSetupMarkFailed):
		// Nothing changed on the router (fail closed); distinct from STATE_NOT_SAVED.
		response.Error(c, http.StatusInternalServerError, "MIKROTIK_SETUP_MARK_FAILED", err.Error())
	case errors.Is(err, mikrotik.ErrSetupStateNotSaved):
		response.Error(c, http.StatusInternalServerError, "MIKROTIK_SETUP_STATE_NOT_SAVED", err.Error())
	case errors.As(err, &stepErr) && errors.Is(err, mikrotik.ErrSetupFailed):
		// Transport causes (unreachable, auth, TLS) were matched above through Unwrap.
		msg := "router rejected setup step (" + stepErr.Step + "); isolir is not marked ready, apply is safe to retry"
		if strings.HasPrefix(stepErr.Step, "remove ") {
			msg = "router rejected setup step (" + stepErr.Step + "); cleanup is incomplete and can be retried"
		}
		response.Error(c, http.StatusBadGateway, "MIKROTIK_SETUP_FAILED", msg)
	case errors.As(err, &appErr) && appErr.Status >= 400 && appErr.Status < 500:
		// Deliberate client-facing refusals from other modules reached through
		// the app adapters (e.g. 409 QUOTA_EXCEEDED when creating a CRM contact).
		response.Error(c, appErr.Status, appErr.Code, appErr.Message)
	default:
		c.Error(err) // logged by middleware, not exposed
		response.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
