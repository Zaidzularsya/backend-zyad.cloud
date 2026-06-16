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

type domainHandlerServiceStub struct {
	organizationID string
	domainID       string
	actorUserID    string
	request        dto.CreateDomainRequest
}

func (s *domainHandlerServiceStub) List(
	_ context.Context,
	organizationID string,
	_ dto.DomainListQuery,
) ([]dto.DomainResponse, dto.PaginationMeta, error) {
	s.organizationID = organizationID
	return []dto.DomainResponse{}, dto.PaginationMeta{
		Page: 1, PerPage: 20,
	}, nil
}

func (s *domainHandlerServiceStub) Create(
	_ context.Context,
	organizationID string,
	request dto.CreateDomainRequest,
	actorUserID string,
) (dto.DomainChallengeResponse, error) {
	s.organizationID = organizationID
	s.actorUserID = actorUserID
	s.request = request
	return dto.DomainChallengeResponse{
		Domain: dto.DomainResponse{ID: handlerOrganizationID},
	}, nil
}

func (s *domainHandlerServiceStub) Verify(
	_ context.Context,
	organizationID string,
	domainID string,
	actorUserID string,
) (dto.DomainResponse, error) {
	s.organizationID = organizationID
	s.domainID = domainID
	s.actorUserID = actorUserID
	return dto.DomainResponse{ID: domainID}, nil
}

func (s *domainHandlerServiceStub) Update(
	context.Context,
	string,
	string,
	dto.UpdateDomainRequest,
	string,
) (dto.DomainResponse, error) {
	return dto.DomainResponse{}, nil
}

func (s *domainHandlerServiceStub) Delete(context.Context, string, string, string) error {
	return nil
}

type domainPermissionCheckerStub struct {
	organizationID string
	permissions    []string
}

func (s *domainPermissionCheckerStub) CanOrganization(
	_ context.Context,
	_ string,
	organizationID string,
	permissions []string,
) error {
	s.organizationID = organizationID
	s.permissions = append(s.permissions, permissions...)
	return nil
}

func TestDomainHandlerUsesVerifiedOrganizationContext(t *testing.T) {
	service := &domainHandlerServiceStub{}
	checker := &domainPermissionCheckerStub{}
	router := domainHandlerRouter(t, service, checker)

	request := httptest.NewRequest(
		http.MethodPost,
		"/organization/domains",
		bytes.NewBufferString(`{
			"type":"custom",
			"canonical_host":"www.example.com"
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.organizationID != handlerOrganizationID ||
		service.actorUserID != handlerUserID ||
		checker.organizationID != handlerOrganizationID ||
		len(checker.permissions) != 1 ||
		checker.permissions[0] != "organization.domain.manage" {
		t.Fatalf(
			"service organization=%q checker=%#v",
			service.organizationID,
			checker,
		)
	}
}

func TestDomainHandlerVerifyScopesDomainToCurrentOrganization(t *testing.T) {
	service := &domainHandlerServiceStub{}
	router := domainHandlerRouter(
		t,
		service,
		&domainPermissionCheckerStub{},
	)
	domainID := "55555555-5555-5555-5555-555555555555"

	recorder := httptest.NewRecorder()
	router.ServeHTTP(
		recorder,
		httptest.NewRequest(
			http.MethodPost,
			"/organization/domains/"+domainID+"/verify",
			nil,
		),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.organizationID != handlerOrganizationID ||
		service.domainID != domainID ||
		service.actorUserID != handlerUserID {
		t.Fatalf(
			"organization=%q domain=%q",
			service.organizationID,
			service.domainID,
		)
	}
}

func domainHandlerRouter(
	t *testing.T,
	service OrganizationDomainService,
	checker permissionmiddleware.OrganizationPermissionChecker,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		tenantContext, err := coretenant.NewVerifiedContext(
			coretenant.VerifiedContextInput{
				OrganizationID:     handlerOrganizationID,
				OrganizationSlug:   "acme",
				OrganizationType:   coretenant.OrganizationTypeCustomer,
				OrganizationStatus: coretenant.OrganizationStatusActive,
				MembershipID:       handlerMembershipID,
				MembershipStatus:   "active",
				MembershipVersion:  1,
				ResolutionSource:   coretenant.ResolutionSourceSession,
				DataPlacement:      coretenant.DataPlacementShared,
			},
		)
		if err != nil {
			t.Fatalf("create tenant context: %v", err)
		}
		middleware.SetTenantContext(c, tenantContext)
		permissionmiddleware.SetUserID(c, handlerUserID)
		c.Next()
	})
	group := router.Group("")
	NewDomainHandler(service, checker).RegisterRoutes(group)
	return router
}
