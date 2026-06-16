package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	coreerrors "zyad.cloud/internal/core/errors"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/organization/dto"

	"github.com/gin-gonic/gin"
)

type fakePlatformHandlerService struct {
	query     dto.OrganizationListQuery
	request   dto.CreateOrganizationRequest
	actorID   string
	listCalls int
}

func (f *fakePlatformHandlerService) List(
	_ context.Context,
	query dto.OrganizationListQuery,
) ([]dto.OrganizationResponse, dto.PaginationMeta, error) {
	f.listCalls++
	f.query = query
	return []dto.OrganizationResponse{{ID: handlerOrganizationID}},
		dto.PaginationMeta{Page: 1, PerPage: 20, Total: 1, TotalPages: 1}, nil
}

func (f *fakePlatformHandlerService) Get(
	context.Context,
	string,
) (dto.PlatformOrganizationDetailResponse, error) {
	return dto.PlatformOrganizationDetailResponse{
		Organization: dto.OrganizationResponse{ID: handlerOrganizationID},
	}, nil
}

func (f *fakePlatformHandlerService) Create(
	_ context.Context,
	request dto.CreateOrganizationRequest,
	actorID string,
) (dto.OrganizationResponse, error) {
	f.request = request
	f.actorID = actorID
	return dto.OrganizationResponse{ID: handlerOrganizationID}, nil
}

func (f *fakePlatformHandlerService) Update(
	context.Context,
	string,
	dto.UpdateOrganizationRequest,
) (dto.OrganizationResponse, error) {
	return dto.OrganizationResponse{ID: handlerOrganizationID}, nil
}

func (f *fakePlatformHandlerService) ChangeStatus(
	context.Context,
	string,
	dto.UpdateOrganizationStatusRequest,
	string,
) (dto.OrganizationResponse, error) {
	return dto.OrganizationResponse{ID: handlerOrganizationID}, nil
}

func (f *fakePlatformHandlerService) Provision(
	_ context.Context,
	_ string,
	actorID string,
) (dto.PlatformOrganizationDetailResponse, error) {
	f.actorID = actorID
	return dto.PlatformOrganizationDetailResponse{
		Organization: dto.OrganizationResponse{ID: handlerOrganizationID},
	}, nil
}

type platformPermissionChecker struct {
	permissions []string
	err         error
}

func (c *platformPermissionChecker) Can(
	_ context.Context,
	_ string,
	permissions []string,
) error {
	c.permissions = append(c.permissions, permissions...)
	return c.err
}

func TestPlatformHandlerListRequiresPlatformContextAndPermission(t *testing.T) {
	service := &fakePlatformHandlerService{}
	checker := &platformPermissionChecker{}
	router := platformHandlerRouter(t, service, checker, coretenant.OrganizationTypePlatform)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/platform/organizations?page=2", nil),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.query.Page != 2 ||
		len(checker.permissions) != 1 ||
		checker.permissions[0] != "platform.organization.read" {
		t.Fatalf("query=%#v permissions=%#v", service.query, checker.permissions)
	}
}

func TestPlatformHandlerRejectsCustomerContext(t *testing.T) {
	router := platformHandlerRouter(
		t,
		&fakePlatformHandlerService{},
		&platformPermissionChecker{},
		coretenant.OrganizationTypeCustomer,
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/platform/organizations", nil),
	)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPlatformHandlerRejectsPlatformContextWithoutExplicitPermission(t *testing.T) {
	service := &fakePlatformHandlerService{}
	checker := &platformPermissionChecker{
		err: coreerrors.New(
			"PERMISSION_DENIED",
			"permission denied",
			http.StatusForbidden,
		),
	}
	router := platformHandlerRouter(
		t,
		service,
		checker,
		coretenant.OrganizationTypePlatform,
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/platform/organizations", nil),
	)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.listCalls != 0 ||
		len(checker.permissions) != 1 ||
		checker.permissions[0] != "platform.organization.read" {
		t.Fatalf(
			"listCalls=%d permissions=%#v",
			service.listCalls,
			checker.permissions,
		)
	}
}

func TestPlatformHandlerCreateUsesAuthenticatedActor(t *testing.T) {
	service := &fakePlatformHandlerService{}
	router := platformHandlerRouter(
		t,
		service,
		&platformPermissionChecker{},
		coretenant.OrganizationTypePlatform,
	)
	request := httptest.NewRequest(
		http.MethodPost,
		"/platform/organizations",
		bytes.NewBufferString(`{
			"type":"customer",
			"slug":"acme",
			"name":"Acme",
			"owner_user_id":"11111111-1111-1111-1111-111111111111"
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.actorID != handlerUserID || service.request.Slug != "acme" {
		t.Fatalf("actor=%q request=%#v", service.actorID, service.request)
	}
}

func TestPlatformHandlerProvisionRequiresPermission(t *testing.T) {
	service := &fakePlatformHandlerService{}
	checker := &platformPermissionChecker{}
	router := platformHandlerRouter(
		t,
		service,
		checker,
		coretenant.OrganizationTypePlatform,
	)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/platform/organizations/"+handlerOrganizationID+"/provision",
			nil,
		),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.actorID != handlerUserID ||
		len(checker.permissions) != 1 ||
		checker.permissions[0] != "platform.organization.provision" {
		t.Fatalf("actor=%q permissions=%#v", service.actorID, checker.permissions)
	}
}

func platformHandlerRouter(
	t *testing.T,
	service PlatformOrganizationService,
	checker permissionmiddleware.PermissionChecker,
	organizationType coretenant.OrganizationType,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
			OrganizationID:     handlerOrganizationID,
			OrganizationSlug:   "platform",
			OrganizationType:   organizationType,
			OrganizationStatus: coretenant.OrganizationStatusActive,
			ResolutionSource:   coretenant.ResolutionSourcePlatformHost,
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
	NewPlatformHandler(service, checker).RegisterRoutes(group)
	return router
}
