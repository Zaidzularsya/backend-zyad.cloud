package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/catalog/dto"
)

type PublicListingService interface {
	Listings(ctx context.Context) (dto.PublicCatalogResponse, error)
	TenantListings(ctx context.Context, scope coretenant.Scope) (dto.PublicCatalogResponse, error)
}

// PublicListingHandler menyajikan produk publik org platform tanpa auth.
type PublicListingHandler struct{ svc PublicListingService }

func NewPublicListingHandler(svc PublicListingService) *PublicListingHandler {
	return &PublicListingHandler{svc: svc}
}

func (h *PublicListingHandler) RegisterRoutes(router *gin.RouterGroup) {
	router.GET("/public/catalog/listings", h.List)
}

func (h *PublicListingHandler) List(c *gin.Context) {
	result, err := h.svc.Listings(c.Request.Context())
	if err != nil {
		failCatalog(c, err)
		return
	}
	c.Header("Cache-Control", "public, max-age=60")
	corehttp.OK(c, "success", result)
}

// RegisterTenantRoutes memasang listing tenant. Router WAJIB berada di grup publik
// yang tenant-nya di-resolve dari host (middleware.ResolvePublicOrganization),
// sama seperti /public/landing/*; id organisasi tidak pernah dibaca dari klien.
func (h *PublicListingHandler) RegisterTenantRoutes(router gin.IRoutes) {
	router.GET("/public/tenant-catalog", h.TenantList)
}

func (h *PublicListingHandler) TenantList(c *gin.Context) {
	tc, err := coretenant.RequireContext(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	// Host platform memakai /public/catalog/listings.
	if tc.OrganizationType() == coretenant.OrganizationTypePlatform {
		corehttp.Fail(c, coreerrors.New("TENANT_CATALOG_NOT_FOUND", "katalog tenant tidak ditemukan untuk host ini", http.StatusNotFound))
		return
	}
	scope, err := coretenant.NewScope(tc)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	result, err := h.svc.TenantListings(c.Request.Context(), scope)
	if err != nil {
		failCatalog(c, err)
		return
	}
	// Respons bergantung pada host (dan X-Forwarded-Host bila dipercaya middleware):
	// cache bersama wajib memisahkan per host agar katalog tenant A tak sampai ke B.
	c.Header("Vary", "Host, X-Forwarded-Host")
	c.Header("Cache-Control", "public, max-age=60")
	corehttp.OK(c, "success", result)
}

// RegisterAppRoutes memasang listing milik org yang sedang login. Dipakai editor landing
// (pratinjau kanvas + panel blok Pricing Katalog) yang berjalan di host platform, sehingga
// tidak bisa memakai /public/tenant-catalog yang menentukan tenant dari host.
func (h *PublicListingHandler) RegisterAppRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	router.GET("/listings", permissionmiddleware.RequireOrganizationOrGlobal(p, "catalog_product.read"), h.AppList)
}

func (h *PublicListingHandler) AppList(c *gin.Context) {
	tc, err := coretenant.RequireContext(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	var result dto.PublicCatalogResponse
	if tc.OrganizationType() == coretenant.OrganizationTypePlatform {
		result, err = h.svc.Listings(c.Request.Context())
	} else {
		var scope coretenant.Scope
		if scope, err = coretenant.NewScope(tc); err == nil {
			result, err = h.svc.TenantListings(c.Request.Context(), scope)
		}
	}
	if err != nil {
		failCatalog(c, err)
		return
	}
	// Respons per-org dan butuh login: jangan masuk cache bersama.
	c.Header("Cache-Control", "private, no-store")
	corehttp.OK(c, "success", result)
}
