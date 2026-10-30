package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/catalog/dto"
	"zyad.cloud/internal/modules/catalog/service"
)

// FeatureHandler menyajikan registry fitur aktif untuk form produk (tenant: hanya yang offerable).
type FeatureHandler struct {
	svc     service.ProductService
	checker permissionmiddleware.CombinedPermissionChecker
}

func NewFeatureHandler(svc service.ProductService) *FeatureHandler { return &FeatureHandler{svc: svc} }

func (h *FeatureHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	h.checker = p
	router.GET("/features", permissionmiddleware.RequireOrganizationOrGlobal(p, "catalog_product.update"), h.List)
}

func (h *FeatureHandler) List(c *gin.Context) {
	platform := isPlatformOrg(c)
	if !platform && !hasManageEntitlements(c, h.checker) {
		corehttp.Fail(c, coreerrors.New("FORBIDDEN", "insufficient permissions", http.StatusForbidden))
		return
	}
	defs, err := h.svc.ListFeatures(c.Request.Context())
	if err != nil {
		failCatalog(c, err)
		return
	}
	out := make([]dto.FeatureDefResponse, 0, len(defs))
	for _, d := range defs {
		r := dto.FeatureDefResponse{Key: d.Key, Name: d.Name, Module: d.Module, ValueType: d.ValueType, Unit: d.Unit, ResetStrategy: d.ResetStrategy}
		if !platform {
			// Tenant hanya melihat fitur yang boleh ditawarkan, lengkap dengan batasnya.
			if !d.TenantOfferable {
				continue
			}
			r.TenantMax = d.TenantMax
		}
		out = append(out, r)
	}
	corehttp.OK(c, "success", out)
}
