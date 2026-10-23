package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"zyad.cloud/internal/core/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/catalog/dto"
)

const (
	hostOrgA = "11111111-1111-1111-1111-111111111111"
	hostOrgP = "99999999-9999-9999-9999-999999999999"
)

// hostResolver meniru PublicHostResolver: host -> org (tenant A / platform), lainnya tak dikenal.
type hostResolver struct{ t *testing.T }

func (h hostResolver) ResolvePublicHost(_ context.Context, host string) (coretenant.Context, bool, error) {
	var id, slug string
	var typ coretenant.OrganizationType
	switch host {
	case "a.example.com":
		id, slug, typ = hostOrgA, "org-a", coretenant.OrganizationTypeCustomer
	case "platform.example.com":
		id, slug, typ = hostOrgP, "platform", coretenant.OrganizationTypePlatform
	default:
		return coretenant.Context{}, false, nil
	}
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: id, OrganizationSlug: slug, OrganizationType: typ, OrganizationStatus: coretenant.OrganizationStatusActive,
		MembershipID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", MembershipStatus: "active", MembershipVersion: 1,
		ResolutionSource: coretenant.ResolutionSourceCustomDomain, DataPlacement: coretenant.DataPlacementShared, RequestHost: host,
	})
	if err != nil {
		h.t.Fatal(err)
	}
	return tc, true, nil
}

type fakeListingSvc struct{ scopes []string }

func (f *fakeListingSvc) Listings(context.Context) (dto.PublicCatalogResponse, error) {
	return dto.PublicCatalogResponse{Categories: []dto.PublicCategory{{ID: "platform"}}}, nil
}

func (f *fakeListingSvc) TenantListings(_ context.Context, s coretenant.Scope) (dto.PublicCatalogResponse, error) {
	f.scopes = append(f.scopes, s.OrganizationID())
	return dto.PublicCatalogResponse{Categories: []dto.PublicCategory{{ID: "cat-of-" + s.OrganizationID()}}}, nil
}

func listingRouter(t *testing.T, svc *fakeListingSvc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	mw, err := middleware.ResolvePublicOrganization(hostResolver{t}, middleware.PublicHostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	h := NewPublicListingHandler(svc)
	h.RegisterRoutes(r.Group("")) // platform: tanpa resolusi host
	h.RegisterTenantRoutes(r.Group("", mw))
	return r
}

func varyOf(r http.Handler, host, path string) string {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Header().Get("Vary")
}

func TestTenantCatalogVaryOnHost(t *testing.T) {
	v := varyOf(listingRouter(t, &fakeListingSvc{}), "a.example.com", "/public/tenant-catalog")
	if !strings.Contains(v, "Host") || !strings.Contains(v, "X-Forwarded-Host") {
		t.Fatalf("Vary = %q", v)
	}
}

func getHost(r http.Handler, host, path string) (int, string) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func TestTenantCatalogUsesHostOrganization(t *testing.T) {
	svc := &fakeListingSvc{}
	r := listingRouter(t, svc)
	// organization_id dari klien harus diabaikan; tenant hanya dari host.
	code, body := getHost(r, "a.example.com", "/public/tenant-catalog?organization_id=other")
	if code != http.StatusOK || !strings.Contains(body, "cat-of-"+hostOrgA) {
		t.Fatalf("code=%d body=%s", code, body)
	}
	for _, id := range svc.scopes {
		if id != hostOrgA {
			t.Fatalf("scope org = %s", id)
		}
	}
}

func TestTenantCatalogUnknownHost404(t *testing.T) {
	svc := &fakeListingSvc{}
	code, _ := getHost(listingRouter(t, svc), "unknown.example.com", "/public/tenant-catalog")
	if code != http.StatusNotFound || len(svc.scopes) != 0 {
		t.Fatalf("code=%d calls=%v", code, svc.scopes)
	}
}

func TestTenantCatalogPlatformHost404(t *testing.T) {
	svc := &fakeListingSvc{}
	code, _ := getHost(listingRouter(t, svc), "platform.example.com", "/public/tenant-catalog")
	if code != http.StatusNotFound || len(svc.scopes) != 0 {
		t.Fatalf("code=%d calls=%v", code, svc.scopes)
	}
}

func TestPlatformListingEndpointUnchanged(t *testing.T) {
	svc := &fakeListingSvc{}
	code, body := getHost(listingRouter(t, svc), "anything.example.com", "/public/catalog/listings")
	if code != http.StatusOK || !strings.Contains(body, `"platform"`) || len(svc.scopes) != 0 {
		t.Fatalf("code=%d body=%s", code, body)
	}
}
