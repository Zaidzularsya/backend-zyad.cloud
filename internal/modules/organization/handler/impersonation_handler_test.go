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
	"zyad.cloud/internal/modules/organization/service"

	"github.com/gin-gonic/gin"
)

type impersonationHandlerServiceStub struct {
	operatorUserID       string
	operatorSessionID    string
	targetOrganizationID string
	startRequest         dto.StartImpersonationRequest
	stopRequest          dto.StopImpersonationRequest
}

func (s *impersonationHandlerServiceStub) Start(
	_ context.Context,
	operatorUserID string,
	operatorSessionID string,
	targetOrganizationID string,
	request dto.StartImpersonationRequest,
	_ service.ImpersonationMetadata,
) (dto.ImpersonationSessionResponse, error) {
	s.operatorUserID = operatorUserID
	s.operatorSessionID = operatorSessionID
	s.targetOrganizationID = targetOrganizationID
	s.startRequest = request
	return dto.ImpersonationSessionResponse{ID: handlerSessionID}, nil
}

func (s *impersonationHandlerServiceStub) Stop(
	_ context.Context,
	operatorUserID string,
	operatorSessionID string,
	request dto.StopImpersonationRequest,
	_ service.ImpersonationMetadata,
) (dto.ImpersonationSessionResponse, error) {
	s.operatorUserID = operatorUserID
	s.operatorSessionID = operatorSessionID
	s.stopRequest = request
	return dto.ImpersonationSessionResponse{ID: handlerSessionID}, nil
}

func TestImpersonationHandlerStartUsesAuthenticatedOperatorSession(t *testing.T) {
	service := &impersonationHandlerServiceStub{}
	checker := &platformPermissionChecker{}
	router := impersonationHandlerRouter(t, service, checker)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/platform/organizations/"+handlerOrganizationID+"/impersonate",
		bytes.NewBufferString(`{
			"target_user_id":"55555555-5555-5555-5555-555555555555",
			"reason":"support ticket investigation",
			"ticket_reference":"TICKET-123",
			"expires_at":"2026-01-01T01:00:00Z"
		}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.operatorUserID != handlerUserID ||
		service.operatorSessionID != handlerSessionID ||
		service.targetOrganizationID != handlerOrganizationID ||
		service.startRequest.TicketReference != "TICKET-123" ||
		len(checker.permissions) != 1 ||
		checker.permissions[0] != "platform.organization.impersonate" {
		t.Fatalf("service=%#v permissions=%#v", service, checker.permissions)
	}
}

func TestImpersonationHandlerStopUsesAuthenticatedOperatorSession(t *testing.T) {
	service := &impersonationHandlerServiceStub{}
	checker := &platformPermissionChecker{}
	router := impersonationHandlerRouter(t, service, checker)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodDelete,
		"/platform/impersonation",
		bytes.NewBufferString(`{"reason":"support complete"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if service.operatorUserID != handlerUserID ||
		service.operatorSessionID != handlerSessionID ||
		service.stopRequest.Reason != "support complete" ||
		len(checker.permissions) != 1 ||
		checker.permissions[0] != "platform.organization.impersonate" {
		t.Fatalf("service=%#v permissions=%#v", service, checker.permissions)
	}
}

func impersonationHandlerRouter(
	t *testing.T,
	service OrganizationImpersonationService,
	checker permissionmiddleware.PermissionChecker,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
			OrganizationID:     handlerOrganizationID,
			OrganizationSlug:   "platform",
			OrganizationType:   coretenant.OrganizationTypePlatform,
			OrganizationStatus: coretenant.OrganizationStatusActive,
			ResolutionSource:   coretenant.ResolutionSourcePlatformHost,
			DataPlacement:      coretenant.DataPlacementShared,
		})
		if err != nil {
			t.Fatalf("create tenant context: %v", err)
		}
		middleware.SetTenantContext(c, tenantContext)
		permissionmiddleware.SetUserID(c, handlerUserID)
		c.Set(middleware.AuthenticatedUserContextKey, middleware.AuthenticatedUser{
			ID:        handlerUserID,
			SessionID: handlerSessionID,
		})
		c.Next()
	})
	group := router.Group("")
	NewImpersonationHandler(service, checker).RegisterRoutes(group)
	return router
}
