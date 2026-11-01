package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/mikrotik/dto"
	"zyad.cloud/internal/modules/mikrotik/service"
)

// OnboardingService is the subset of service.OnboardingService used by the handler.
type OnboardingService interface {
	Summary(ctx context.Context, scope coretenant.Scope) (service.OnboardingSummary, error)
	CustomerStatuses(ctx context.Context, scope coretenant.Scope, accountIDs []string) ([]service.CustomerStatus, error)
}

type OnboardingHandler struct {
	svc OnboardingService
}

func NewOnboardingHandler(svc OnboardingService) *OnboardingHandler {
	return &OnboardingHandler{svc: svc}
}

// maxStatusAccounts bounds ?account_ids so one request stays a single cheap query.
const maxStatusAccounts = 100

// RegisterRoutes mounts the read-only summary routes. Neither contacts a
// router, so probeLimit is accepted only to keep the signature uniform.
func (h *OnboardingHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker, _ gin.HandlerFunc) {
	router.GET("/onboarding", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_router.read"), h.Summary)
	router.GET("/customers/status", permissionmiddleware.RequireOrganizationOrGlobal(p, "mikrotik_policy.read"), h.CustomerStatuses)
}

func (h *OnboardingHandler) Summary(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	s, err := h.svc.Summary(c.Request.Context(), scope)
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.OnboardingFromService(s))
}

func (h *OnboardingHandler) CustomerStatuses(c *gin.Context) {
	scope, ok := scopeOrFail(c)
	if !ok {
		return
	}
	ids, err := parseAccountIDs(c.Query("account_ids"))
	if err != nil {
		validationFail(c, err)
		return
	}
	items, err := h.svc.CustomerStatuses(c.Request.Context(), scope, ids)
	if err != nil {
		failRouter(c, err)
		return
	}
	corehttp.OK(c, "success", dto.CustomerStatusesFromService(items))
}

// parseAccountIDs splits a comma list, lowercases and deduplicates it (first
// occurrence wins, so the response keeps request order) and enforces the cap
// on the distinct ids.
func parseAccountIDs(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("account_ids is required")
	}
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	ids := make([]string, 0, len(parts))
	for _, part := range parts {
		id := strings.ToLower(strings.TrimSpace(part))
		if !validUUID(id) {
			return nil, errors.New("account_ids must be a comma-separated list of UUIDs")
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) > maxStatusAccounts {
		return nil, errors.New("account_ids accepts at most 100 ids")
	}
	return ids, nil
}
