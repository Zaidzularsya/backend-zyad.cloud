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

// FeatureHandler menyajikan registry fitur aktif untuk form produk platform.
type FeatureHandler struct {
	svc service.ProductService
}

func NewFeatureHandler(svc service.ProductService) *FeatureHandler { return &FeatureHandler{svc: svc} }

func (h *FeatureHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	router.GET("/features", permissionmiddleware.RequireOrganizationOrGlobal(p, "catalog_product.update"), h.List)
}

func (h *FeatureHandler) List(c *gin.Context) {
	if !isPlatformOrg(c) {
		corehttp.Fail(c, coreerrors.New("PLATFORM_ONLY", "Fitur produk hanya untuk katalog platform", http.StatusForbidden))
		return
	}
	defs, err := h.svc.ListFeatures(c.Request.Context())
	if err != nil {
		failCatalog(c, err)
		return
	}
	out := make([]dto.FeatureDefResponse, 0, len(defs))
	for _, d := range defs {
		out = append(out, dto.FeatureDefResponse{Key: d.Key, Name: d.Name, Module: d.Module, ValueType: d.ValueType, Unit: d.Unit})
	}
	corehttp.OK(c, "success", out)
}
