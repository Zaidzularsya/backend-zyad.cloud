package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/dto"

	"github.com/gin-gonic/gin"
)

type entitlementHandlerServiceStub struct {
	organizationID string
	actorID        string
	request        dto.UpsertEntitlementRequest
	query          dto.UsageQuery
}

func (s *entitlementHandlerServiceStub) ListFeatures(
	_ context.Context,
	organizationID string,
	_ dto.EntitlementListQuery,
) ([]dto.EffectiveFeatureResponse, dto.PaginationMeta, error) {
	s.organizationID = organizationID
	return []dto.EffectiveFeatureResponse{}, dto.PaginationMeta{
		Page: 1, PerPage: 20,
	}, nil
}

func (s *entitlementHandlerServiceStub) CheckUsage(
	_ context.Context,
	organizationID string,
	query dto.UsageQuery,
) (dto.UsageResponse, error) {
	s.organizationID = organizationID
	s.query = query
	return dto.UsageResponse{FeatureKey: query.FeatureKey}, nil
}

func (s *entitlementHandlerServiceStub) UpsertPlatformOverride(
	_ context.Context,
	organizationID string,
	request dto.UpsertEntitlementRequest,
	actorID string,
) (dto.EntitlementResponse, error) {
	s.organizationID = organizationID
	s.actorID = actorID
	s.request = request
	return dto.EntitlementResponse{OrganizationID: organizationID}, nil
}

type entitlementOrganizationCheckerStub struct {
	organizationID string
	permissions    []string
}

func (s *entitlementOrganizationCheckerStub) CanOrganization(
	_ context.Context,
	_ string,
	organizationID string,
	permissions []string,
) error {
	s.organizationID = organizationID
	s.permissions = append(s.permissions, permissions...)
	return nil
}

func TestEntitlementHandlerListFeaturesUsesVerifiedOrganization(t *testing.T) {
	service := &entitlementHandlerServiceStub{}
	checker := &entitlementOrganizationCheckerStub{}
	router := entitlementHandlerRouter(t, service, checker, &platformPermissionChecker{}, coretenant.OrganizationTypeCustomer)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/organization/features?feature_key=landing.pages", nil),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.organizationID != handlerOrganizationID ||
		checker.organizationID != handlerOrganizationID ||
		len(checker.permissions) != 1 ||
		checker.permissions[0] != "organization.feature.read" {
		t.Fatalf("service=%q checker=%#v", service.organizationID, checker)
	}
}

func TestEntitlementHandlerCheckUsageBindsQuery(t *testing.T) {
	service := &entitlementHandlerServiceStub{}
	router := entitlementHandlerRouter(
		t,
		service,
		&entitlementOrganizationCheckerStub{},
		&platformPermissionChecker{},
		coretenant.OrganizationTypeCustomer,
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		httptest.NewRequest(
			http.MethodGet,
			"/organization/usage?feature_key=landing.pages&metric_key=pages&limit_key=max_pages&period_start=2026-01-01T00:00:00Z&period_end=2026-02-01T00:00:00Z",
			nil,
		),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.organizationID != handlerOrganizationID ||
		service.query.MetricKey != "pages" {
		t.Fatalf("organization=%q query=%#v", service.organizationID, service.query)
	}
}

func TestEntitlementHandlerPlatformOverrideUsesPlatformPermissionAndActor(t *testing.T) {
	service := &entitlementHandlerServiceStub{}
	checker := &platformPermissionChecker{}
	router := entitlementHandlerRouter(
		t,
		service,
		&entitlementOrganizationCheckerStub{},
		checker,
		coretenant.OrganizationTypePlatform,
	)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPatch,
		"/platform/organizations/"+handlerOrganizationID+"/entitlements",
		bytes.NewBufferString(`{
			"feature_key":"landing.pages",
			"source":"platform_override",
			"limits":{"max_pages":10},
			"reason":"manual correction"
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.organizationID != handlerOrganizationID ||
		service.actorID != handlerUserID ||
		service.request.FeatureKey != "landing.pages" ||
		len(checker.permissions) != 1 ||
		checker.permissions[0] != "platform.organization.manage" {
		t.Fatalf("service=%#v permissions=%#v", service, checker.permissions)
	}
}

func entitlementHandlerRouter(
	t *testing.T,
	service OrganizationEntitlementService,
	checker permissionmiddleware.OrganizationPermissionChecker,
	platformChecker permissionmiddleware.PermissionChecker,
	organizationType coretenant.OrganizationType,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
			OrganizationID:     handlerOrganizationID,
			OrganizationSlug:   "acme",
			OrganizationType:   organizationType,
			OrganizationStatus: coretenant.OrganizationStatusActive,
			MembershipID:       handlerMembershipID,
			MembershipStatus:   "active",
			MembershipVersion:  1,
			ResolutionSource:   coretenant.ResolutionSourceSession,
			DataPlacement:      coretenant.DataPlacementShared,
		})
		if err != nil {
			t.Fatalf("create tenant context: %v", err)
		}
		middleware.SetTenantContext(c, tenantContext)
		permissionmiddleware.SetUserID(c, handlerUserID)
		c.Next()
	})
	group := router.Group("")
	NewEntitlementHandler(service, checker, platformChecker).RegisterRoutes(group)
	return router
}
