package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	coretenant "zyad.cloud/internal/core/tenant"
)

// newTestRouter injects a verified tenant context so handlers can call
// coretenant.RequireScope. Permission middleware is bypassed on purpose:
// route-level RBAC is asserted separately.
func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tenantContext, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID:     "11111111-1111-1111-1111-111111111111",
		OrganizationSlug:   "organization-a",
		OrganizationType:   coretenant.OrganizationTypeCustomer,
		OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID:       "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		MembershipStatus:   "active",
		MembershipVersion:  1,
		ResolutionSource:   coretenant.ResolutionSourceSession,
		DataPlacement:      coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatalf("tenant context: %v", err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tenantContext))
		c.Next()
	})
	return r
}

func performJSON(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
