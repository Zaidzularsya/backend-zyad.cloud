package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	coretenant "zyad.cloud/internal/core/tenant"

	"github.com/gin-gonic/gin"
)

type organizationCheckerStub struct {
	userID         string
	organizationID string
	permissions    []string
}

func (s *organizationCheckerStub) CanOrganization(
	_ context.Context,
	userID string,
	organizationID string,
	permissions []string,
) error {
	s.userID = userID
	s.organizationID = organizationID
	s.permissions = permissions
	return nil
}

func TestRequireOrganizationUsesVerifiedTenantContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &organizationCheckerStub{}
	tenantContext := permissionTenantContext(t)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		SetUserID(c, "user-1")
		c.Request = c.Request.WithContext(
			coretenant.WithContext(c.Request.Context(), tenantContext),
		)
		c.Next()
	})
	router.GET(
		"/resource",
		RequireOrganization(checker, "organization.member.read"),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/resource", nil))

	if recorder.Code != http.StatusNoContent ||
		checker.userID != "user-1" ||
		checker.organizationID != tenantContext.OrganizationID() ||
		len(checker.permissions) != 1 {
		t.Fatalf("response/checker = %d / %#v", recorder.Code, checker)
	}
}

func TestRequireOrganizationRejectsMissingTenantContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	checker := &organizationCheckerStub{}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		SetUserID(c, "user-1")
		c.Next()
	})
	router.GET(
		"/resource",
		RequireOrganization(checker, "organization.member.read"),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/resource", nil))

	if recorder.Code != http.StatusForbidden || checker.organizationID != "" {
		t.Fatalf("response/checker = %d / %#v", recorder.Code, checker)
	}
}

func permissionTenantContext(t *testing.T) coretenant.Context {
	t.Helper()
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     "11111111-1111-1111-1111-111111111111",
		OrganizationSlug:   "acme",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID:       "22222222-2222-2222-2222-222222222222",
		MembershipStatus:   "active",
		MembershipVersion:  1,
		ResolutionSource:   coretenant.ResolutionSourceSession,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("NewVerifiedContext() error = %v", err)
	}
	return tenantContext
}
