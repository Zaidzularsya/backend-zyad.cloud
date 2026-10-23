package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/catalog/domain"
	"zyad.cloud/internal/modules/catalog/repository"
	"zyad.cloud/internal/modules/catalog/service"
)

type denyAll struct{}

func (denyAll) Can(context.Context, string, []string) error { return errors.New("denied") }
func (denyAll) CanOrganization(context.Context, string, string, []string) error {
	return errors.New("denied")
}

func router(t *testing.T) *gin.Engine {
	t.Helper()
	return routerFor(t, coretenant.OrganizationTypeCustomer)
}

func routerFor(t *testing.T, orgType coretenant.OrganizationType) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	tc, err := coretenant.NewVerifiedContext(coretenant.VerifiedContextInput{
		OrganizationID: "11111111-1111-1111-1111-111111111111", OrganizationSlug: "organization-a",
		OrganizationType: orgType, OrganizationStatus: coretenant.OrganizationStatusActive,
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
	code, _ := doBody(r, method, path, body)
	return code
}

func doBody(r http.Handler, method, path, body string) (int, string) {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

type allowAll struct{}

func (allowAll) Can(context.Context, string, []string) error { return nil }
func (allowAll) CanOrganization(context.Context, string, string, []string) error {
	return nil
}

// fakeProductService merekam input yang diterima handler.
type fakeProductService struct {
	service.ProductService
	created  []service.ProductInput
	updated  []repository.UpdateProductParams
	platform []bool
	canMng   []bool
	err      error
	features []domain.FeatureDef
}

func (f *fakeProductService) Create(_ context.Context, _ coretenant.Scope, in service.ProductInput, _ string) (domain.Product, error) {
	f.created = append(f.created, in)
	return domain.Product{ID: "p1", Features: []domain.ProductFeature{{FeatureKey: "crm", Value: json.RawMessage(`true`), Label: "CRM"}}}, f.err
}

func (f *fakeProductService) Update(_ context.Context, _ coretenant.Scope, id string, in repository.UpdateProductParams, platform, canManage bool) (domain.Product, error) {
	f.canMng = append(f.canMng, canManage)
	f.updated = append(f.updated, in)
	f.platform = append(f.platform, platform)
	return domain.Product{ID: id}, f.err
}

func (f *fakeProductService) ListFeatures(context.Context) ([]domain.FeatureDef, error) {
	return f.features, nil
}

func productRouter(t *testing.T, orgType coretenant.OrganizationType, svc *fakeProductService) *gin.Engine {
	return productRouterWith(t, orgType, svc, allowAll{})
}

func productRouterWith(t *testing.T, orgType coretenant.OrganizationType, svc *fakeProductService, p permissionmiddleware.CombinedPermissionChecker) *gin.Engine {
	r := routerFor(t, orgType)
	g := r.Group("")
	NewProductHandler(svc).RegisterRoutes(g, p)
	NewFeatureHandler(svc).RegisterRoutes(g, p)
	return r
}

// denyEntitlements mengizinkan semua permission kecuali catalog_product.manage_entitlements.
type denyEntitlements struct{}

func (denyEntitlements) Can(_ context.Context, _ string, perms []string) error {
	return denyEntitlements{}.CanOrganization(nil, "", "", perms)
}
func (denyEntitlements) CanOrganization(_ context.Context, _, _ string, perms []string) error {
	for _, p := range perms {
		if p == "catalog_product.manage_entitlements" {
			return errors.New("denied")
		}
	}
	return nil
}

const featureBody = `{"name":"A","features":[{"feature_key":"crm","value":true,"display_label":"CRM lengkap","position":3},{"feature_key":"users","value":5.0}]}`

func TestCreateProductCustomerGetsPlatformFalseAnd422(t *testing.T) {
	svc := &fakeProductService{err: service.ErrPlatformOnly}
	r := productRouter(t, coretenant.OrganizationTypeCustomer, svc)
	code, body := doBody(r, http.MethodPost, "/products", featureBody)
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, "VALIDATION_ERROR") {
		t.Fatalf("code=%d body=%s", code, body)
	}
	if len(svc.created) != 1 || svc.created[0].Platform {
		t.Fatalf("service input = %+v", svc.created)
	}
}

func TestCreateProductPlatformForwardsFeaturesVerbatim(t *testing.T) {
	svc := &fakeProductService{}
	r := productRouter(t, coretenant.OrganizationTypePlatform, svc)
	code, body := doBody(r, http.MethodPost, "/products", featureBody)
	if code != http.StatusOK || !strings.Contains(body, `"label":"CRM"`) {
		t.Fatalf("code=%d body=%s", code, body)
	}
	in := svc.created[0]
	if !in.Platform || len(in.Features) != 2 || string(in.Features[0].Value) != `true` || string(in.Features[1].Value) != `5.0` ||
		in.Features[0].DisplayLabel != "CRM lengkap" || in.Features[0].Position != 3 {
		t.Fatalf("service input = %+v", in)
	}
}

func TestUpdateProductPartialListingIs422WithoutService(t *testing.T) {
	svc := &fakeProductService{}
	r := productRouter(t, coretenant.OrganizationTypePlatform, svc)
	if code := do(r, http.MethodPatch, "/products/p1", `{"listing_code":"free"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("code = %d", code)
	}
	if len(svc.updated) != 0 {
		t.Fatal("service tidak boleh dipanggil")
	}
	if code := do(r, http.MethodPatch, "/products/p1", `{"is_public":true,"listing_code":"free","listing_order":1,"features":[]}`); code != http.StatusOK {
		t.Fatalf("blok lengkap code = %d", code)
	}
	if len(svc.updated) != 1 || !svc.platform[0] || svc.updated[0].Listing == nil || svc.updated[0].Features == nil || len(*svc.updated[0].Features) != 0 {
		t.Fatalf("updated = %+v", svc.updated)
	}
	if code := do(r, http.MethodPatch, "/products/p1", `{"name":"B"}`); code != http.StatusOK {
		t.Fatal("update biasa gagal")
	}
	if got := svc.updated[1]; got.Listing != nil || got.Features != nil {
		t.Fatalf("listing/features harus nil bila tidak dikirim: %+v", got)
	}
}

func TestListFeatures_TenantOnlyOfferableWithMax(t *testing.T) {
	ten := 10
	svc := &fakeProductService{features: []domain.FeatureDef{
		{Key: "crm", Name: "CRM", ValueType: "boolean", ResetStrategy: "never", TenantOfferable: true},
		{Key: "users", Name: "User", ValueType: "integer", ResetStrategy: "never", TenantOfferable: true, TenantMax: &ten},
		{Key: "internal", Name: "Internal", ValueType: "boolean", ResetStrategy: "never"},
	}}
	code, body := doBody(productRouter(t, coretenant.OrganizationTypeCustomer, svc), http.MethodGet, "/features", "")
	if code != http.StatusOK || strings.Contains(body, `"internal"`) || !strings.Contains(body, `"key":"crm"`) || !strings.Contains(body, `"tenant_max":10`) {
		t.Fatalf("code=%d body=%s", code, body)
	}
	code, body = doBody(productRouterWith(t, coretenant.OrganizationTypeCustomer, svc, denyEntitlements{}), http.MethodGet, "/features", "")
	if code != http.StatusForbidden || !strings.Contains(body, "FORBIDDEN") {
		t.Fatalf("tanpa permission code=%d body=%s", code, body)
	}
	code, body = doBody(productRouter(t, coretenant.OrganizationTypePlatform, svc), http.MethodGet, "/features", "")
	if code != http.StatusOK || !strings.Contains(body, `"key":"internal"`) {
		t.Fatalf("platform code=%d body=%s", code, body)
	}
}

func TestProductHandlerForwardsCanManageEntitlements(t *testing.T) {
	svc := &fakeProductService{}
	r := productRouter(t, coretenant.OrganizationTypeCustomer, svc)
	do(r, http.MethodPost, "/products", featureBody)
	do(r, http.MethodPatch, "/products/p1", `{"features":[]}`)
	if !svc.created[0].CanManageEntitlements || !svc.canMng[0] {
		t.Fatalf("flag harus true: %+v %v", svc.created, svc.canMng)
	}
	svc = &fakeProductService{}
	r = productRouterWith(t, coretenant.OrganizationTypeCustomer, svc, denyEntitlements{})
	do(r, http.MethodPost, "/products", featureBody)
	do(r, http.MethodPatch, "/products/p1", `{"name":"B"}`)
	if svc.created[0].CanManageEntitlements || svc.canMng[0] {
		t.Fatalf("flag harus false: %+v %v", svc.created, svc.canMng)
	}
}

func TestNewFeatureErrorsMapTo422(t *testing.T) {
	for _, e := range []error{service.ErrFeatureNotOfferable, service.ErrFeatureAboveMax} {
		svc := &fakeProductService{err: e}
		code, body := doBody(productRouter(t, coretenant.OrganizationTypeCustomer, svc), http.MethodPost, "/products", `{"name":"A"}`)
		if code != http.StatusUnprocessableEntity || !strings.Contains(body, "VALIDATION_ERROR") {
			t.Errorf("%v: code=%d body=%s", e, code, body)
		}
	}
}

func TestListFeaturesPlatformUnchanged(t *testing.T) {
	svc := &fakeProductService{features: []domain.FeatureDef{{Key: "crm", Name: "CRM", ValueType: "boolean", ResetStrategy: "never"}}}
	code, body := doBody(productRouter(t, coretenant.OrganizationTypePlatform, svc), http.MethodGet, "/features", "")
	if code != http.StatusOK || !strings.Contains(body, `"value_type":"boolean"`) || !strings.Contains(body, `"reset_strategy":"never"`) {
		t.Fatalf("platform code=%d body=%s", code, body)
	}
}

func TestListingExistsMapsTo409(t *testing.T) {
	svc := &fakeProductService{err: repository.ErrListingExists}
	r := productRouter(t, coretenant.OrganizationTypePlatform, svc)
	code, body := doBody(r, http.MethodPost, "/products", `{"name":"A"}`)
	if code != http.StatusConflict || !strings.Contains(body, "PRODUCT_LISTING_EXISTS") {
		t.Fatalf("code=%d body=%s", code, body)
	}
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
