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
	"zyad.cloud/internal/modules/mikrotik/dto"
	"zyad.cloud/internal/modules/mikrotik/service"
	"zyad.cloud/internal/shared/response"
)

// SecretOnboardingService is the subset of service.SecretOnboardingService used by the handler.
type SecretOnboardingService interface {
	Preview(ctx context.Context, scope coretenant.Scope, in service.OnboardInput, actor string) (service.OnboardPlan, error)
	Run(ctx context.Context, scope coretenant.Scope, in service.OnboardInput, actor string) (service.OnboardState, error)
	State(ctx context.Context, scope coretenant.Scope, secretID string) (service.OnboardState, bool, error)
}

type SecretOnboardingHandler struct{ svc SecretOnboardingService }

func NewSecretOnboardingHandler(svc SecretOnboardingService) *SecretOnboardingHandler {
	return &SecretOnboardingHandler{svc: svc}
}

// RegisterRoutes mounts the onboarding routes. The CRM/receivable permissions
// of each chain step are checked by the service before anything is written.
func (h *SecretOnboardingHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker, _ gin.HandlerFunc) {
	router.POST("/secrets/:id/onboard/preview", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_secret.pair"), h.Preview)
	router.POST("/secrets/:id/onboard", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_secret.pair"), h.Run)
	router.GET("/secrets/:id/onboard", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_secret.read"), h.Get)
}

func (h *SecretOnboardingHandler) bind(c *gin.Context) (coretenant.Scope, service.OnboardInput, bool) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return scope, service.OnboardInput{}, false
	}
	id := c.Param("id")
	if !validUUID(id) {
		failOnboarding(c, mikrotik.ErrSecretNotFound)
		return scope, service.OnboardInput{}, false
	}
	var req dto.OnboardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Isi permintaan tidak valid.")
		return scope, service.OnboardInput{}, false
	}
	return scope, req.Input(id), true
}

func (h *SecretOnboardingHandler) Preview(c *gin.Context) {
	scope, in, ok := h.bind(c)
	if !ok {
		return
	}
	plan, err := h.svc.Preview(c.Request.Context(), scope, in, permissionmiddleware.UserID(c))
	if err != nil {
		failOnboarding(c, err)
		return
	}
	corehttp.OK(c, "success", dto.OnboardPlanFromService(plan))
}

func (h *SecretOnboardingHandler) Run(c *gin.Context) {
	extendWriteDeadline(c, probeWriteDeadline)
	scope, in, ok := h.bind(c)
	if !ok {
		return
	}
	state, err := h.svc.Run(c.Request.Context(), scope, in, permissionmiddleware.UserID(c))
	if err != nil {
		failOnboarding(c, err)
		return
	}
	corehttp.OK(c, "success", dto.OnboardStateFromService(state))
}

func (h *SecretOnboardingHandler) Get(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if !validUUID(id) {
		failOnboarding(c, mikrotik.ErrSecretNotFound)
		return
	}
	state, found, err := h.svc.State(c.Request.Context(), scope, id)
	if err != nil {
		failOnboarding(c, err)
		return
	}
	if !found {
		corehttp.OK(c, "success", nil)
		return
	}
	corehttp.OK(c, "success", dto.OnboardStateFromService(state))
}

func failOnboarding(c *gin.Context, err error) {
	var dup *service.OnboardDuplicateError
	switch {
	case errors.As(err, &dup):
		response.ErrorWithData(c, http.StatusConflict, "CONTACT_DUPLICATE",
			"Kontak dengan email atau nomor WhatsApp yang sama sudah ada.",
			gin.H{"existing": dto.OnboardPlanFromService(service.OnboardPlan{Contact: dup.Existing}).Contact})
	case errors.Is(err, mikrotik.ErrOnboardingInProgress):
		response.Error(c, http.StatusConflict, "MIKROTIK_ONBOARDING_IN_PROGRESS", "Onboarding secret ini sedang berjalan. Tunggu sampai selesai.")
	case errors.Is(err, mikrotik.ErrOnboardingForbidden):
		missing := strings.TrimPrefix(err.Error(), mikrotik.ErrOnboardingForbidden.Error()+": ")
		response.Error(c, http.StatusForbidden, "MIKROTIK_ONBOARDING_FORBIDDEN", "Anda belum punya izin untuk langkah onboarding: "+missing+".")
	case errors.Is(err, mikrotik.ErrSecretHasContract):
		response.Error(c, http.StatusConflict, "MIKROTIK_SECRET_HAS_CONTRACT", "Pelanggan secret ini sudah punya kontrak.")
	case errors.Is(err, mikrotik.ErrContactNotFound):
		response.Error(c, http.StatusNotFound, "CONTACT_NOT_FOUND", "Kontak tidak ditemukan.")
	default:
		failRouter(c, err)
	}
}
