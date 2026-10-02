package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
)

type denyAll struct{}

func (denyAll) Can(context.Context, string, []string) error { return errors.New("denied") }
func (denyAll) CanOrganization(context.Context, string, string, []string) error {
	return errors.New("denied")
}

func router(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: "11111111-1111-1111-1111-111111111111", OrganizationSlug: "organization-a",
		OrganizationType: coretenant.OrganizationTypeCustomer, OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", MembershipStatus: "active", MembershipVersion: 1,
		ResolutionSource: coretenant.ResolutionSourceSession, DataPlacement: coretenant.DataPlacementShared,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(coretenant.WithContext(c.Request.Context(), tc))
		permissionmiddleware.SetUserID(c, "u1")
		c.Next()
	})
	return r
}

func do(r http.Handler, method, path, body string) int {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestCatalogRoutesRequirePermission(t *testing.T) {
	r := router(t)
	g := r.Group("")
	NewProductHandler(nil).RegisterRoutes(g, denyAll{})
	NewCategoryHandler(nil).RegisterRoutes(g, denyAll{})
	for _, c := range []struct{ m, p string }{
		{http.MethodGet, "/products"}, {http.MethodPost, "/products"}, {http.MethodPatch, "/products/x"},
		{http.MethodDelete, "/products/x"}, {http.MethodGet, "/categories"}, {http.MethodPost, "/categories"},
	} {
		if code := do(r, c.m, c.p, `{}`); code != http.StatusForbidden {
			t.Errorf("%s %s → %d, want 403", c.m, c.p, code)
		}
	}
}
