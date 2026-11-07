package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik"
	"zyad.cloud/internal/modules/mikrotik/domain"
	"zyad.cloud/internal/modules/mikrotik/dto"
	"zyad.cloud/internal/modules/mikrotik/repository"
	"zyad.cloud/internal/modules/mikrotik/service"
	"zyad.cloud/internal/shared/response"
)

// SecretService is the subset of service.SecretService used by the handler.
type SecretService interface {
	Import(ctx context.Context, scope coretenant.Scope, routerID, actorUserID string) (repository.ImportResult, error)
	List(ctx context.Context, scope coretenant.Scope, filter repository.SecretFilter, limit, offset int) ([]domain.PPPSecret, int64, error)
	Pair(ctx context.Context, scope coretenant.Scope, secretID, accountID, actor string) (domain.PPPSecret, error)
	PairNewCustomer(ctx context.Context, scope coretenant.Scope, secretID, actor string) (domain.PPPSecret, error)
	Unpair(ctx context.Context, scope coretenant.Scope, secretID, actor string) error
	SetOriginalProfile(ctx context.Context, scope coretenant.Scope, secretID, profile, actor string) error
	Analyze(ctx context.Context, scope coretenant.Scope, routerID, actor string) (service.AnalyzeResult, error)
	PairBulk(ctx context.Context, scope coretenant.Scope, items []service.PairItem, actor string) []service.PairResult
	SearchAccounts(ctx context.Context, scope coretenant.Scope, q string, limit int) ([]service.AccountRef, error)
}

type SecretHandler struct {
	svc SecretService
}

func NewSecretHandler(svc SecretService) *SecretHandler { return &SecretHandler{svc: svc} }

// RegisterRoutes mounts the PPP secret mapping routes under the parent group
// (tenant + mikrotik.enabled guards). probeLimit (may be nil) runs before the
// permission check on the routes that log in to the tenant's router: import
// and original-profile.
func (h *SecretHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker, probeLimit gin.HandlerFunc) {
	limit := orPass(probeLimit)
	router.POST("/routers/:id/secrets/import", limit, permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_secret.import"), h.Import)
	router.POST("/routers/:id/secrets/analyze", limit, permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_secret.import"), h.Analyze)
	router.POST("/secrets/pair-bulk", limit, permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_secret.pair"), h.PairBulk)
	router.GET("/secrets", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_secret.read"), h.List)
	router.POST("/secrets/:id/pair", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_secret.pair"), h.Pair)
	router.POST("/secrets/:id/pair-new-customer", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_secret.pair"), h.PairNewCustomer)
	router.POST("/secrets/:id/unpair", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_secret.unpair"), h.Unpair)
	router.PUT("/secrets/:id/original-profile", limit, permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_secret.pair"), h.SetOriginalProfile)
	router.GET("/accounts/search", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_secret.read"), h.SearchAccounts)
}

const accountSearchLimit = 20

func (h *SecretHandler) Import(c *gin.Context) {
	extendWriteDeadline(c, probeWriteDeadline)
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	routerID := c.Param("id")
	if !validUUID(routerID) {
		failRouter(c, mikrotik.ErrRouterNotFound)
		return
	}
	res, err := h.svc.Import(c.Request.Context(), scope, routerID, permissionmiddleware.UserID(c))
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.ImportResultFromRepo(res))
}

// Analyze recomputes the pairing suggestions of one router. It reads only the
// local cache and the account directory (no router login) but is heavy, so it
// shares the import rate limit.
func (h *SecretHandler) Analyze(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	routerID := c.Param("id")
	if !validUUID(routerID) {
		failRouter(c, mikrotik.ErrRouterNotFound)
		return
	}
	res, err := h.svc.Analyze(c.Request.Context(), scope, routerID, permissionmiddleware.UserID(c))
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.AnalyzeResultFromService(res))
}

// PairBulk pairs up to 200 secrets. Row failures are reported in the 200
// response; only a malformed request fails as a whole.
func (h *SecretHandler) PairBulk(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var req dto.PairBulkRequest
	if err := bindStrictJSON(c, &req); err != nil {
		validationFail(c, err)
		return
	}
	items := make([]service.PairItem, 0, len(req.Items))
	seen := make(map[string]struct{}, len(req.Items))
	for _, it := range req.Items {
		key := strings.ToLower(it.SecretID)
		if _, dup := seen[key]; dup {
			validationFail(c, errors.New("duplicate secret_id in items"))
			return
		}
		seen[key] = struct{}{}
		items = append(items, service.PairItem{SecretID: it.SecretID, AccountID: it.AccountID})
	}
	results := h.svc.PairBulk(c.Request.Context(), scope, items, permissionmiddleware.UserID(c))
	corehttp.OK(c, "success", dto.PairBulkFromService(results))
}

func (h *SecretHandler) List(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var query dto.SecretListQuery
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
	secrets, total, err := h.svc.List(c.Request.Context(), scope,
		repository.SecretFilter{RouterID: query.RouterID, Paired: query.Paired, Search: query.Q, Suggestion: query.Suggestion, NeedsOriginalProfile: query.NeedsOriginalProfile}, perPage, (page-1)*perPage)
	if err != nil {
		failRouter(c, err)
		return
	}
	response.JSON(c, http.StatusOK, "success", dto.SecretListFromDomain(secrets), dto.BuildMeta(page, perPage, total))
}

func (h *SecretHandler) Pair(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var req dto.PairSecretRequest
	if err := bindStrictJSON(c, &req); err != nil {
		validationFail(c, err)
		return
	}
	id := c.Param("id")
	if !validUUID(id) {
		failRouter(c, mikrotik.ErrSecretNotFound)
		return
	}
	secret, err := h.svc.Pair(c.Request.Context(), scope, id, req.AccountID, permissionmiddleware.UserID(c))
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.SecretFromDomain(secret))
}

func (h *SecretHandler) PairNewCustomer(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if !validUUID(id) {
		failRouter(c, mikrotik.ErrSecretNotFound)
		return
	}
	secret, err := h.svc.PairNewCustomer(c.Request.Context(), scope, id, permissionmiddleware.UserID(c))
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.SecretFromDomain(secret))
}

func (h *SecretHandler) Unpair(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if !validUUID(id) {
		failRouter(c, mikrotik.ErrSecretNotFound)
		return
	}
	if err := h.svc.Unpair(c.Request.Context(), scope, id, permissionmiddleware.UserID(c)); err != nil {
		failRouter(c, err)
		return
	}
	corehttp.NoContent(c)
}

func (h *SecretHandler) SetOriginalProfile(c *gin.Context) {
	extendWriteDeadline(c, probeWriteDeadline)
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var req dto.SetOriginalProfileRequest
	if err := bindStrictJSON(c, &req); err != nil {
		validationFail(c, err)
		return
	}
	id := c.Param("id")
	if !validUUID(id) {
		failRouter(c, mikrotik.ErrSecretNotFound)
		return
	}
	if err := h.svc.SetOriginalProfile(c.Request.Context(), scope, id, req.Profile, permissionmiddleware.UserID(c)); err != nil {
		failRouter(c, err)
		return
	}
	corehttp.NoContent(c)
}

func (h *SecretHandler) SearchAccounts(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	var query dto.AccountSearchQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		validationFail(c, err)
		return
	}
	accounts, err := h.svc.SearchAccounts(c.Request.Context(), scope, query.Q, accountSearchLimit)
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.AccountListFromService(accounts))
}
