package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	"zyad.cloud/internal/modules/asset/service"
)

// PlatformAssetHandler adalah permukaan super-admin-only untuk melihat
// pemakaian storage tenant lain. Untuk MENGUBAH limit-nya, super admin pakai
// endpoint entitlement yang sudah ada,
// PATCH /platform/organizations/:id/entitlements dengan feature_key
// "storage.max_bytes" — tidak diduplikasi di sini.
type PlatformAssetHandler struct {
	assetService service.AssetService
}

func NewPlatformAssetHandler(assetService service.AssetService) *PlatformAssetHandler {
	return &PlatformAssetHandler{assetService: assetService}
}

func (h *PlatformAssetHandler) RegisterRoutes(router *gin.RouterGroup, checker permissionmiddleware.PermissionChecker) {
	group := router.Group("/platform/organizations")
	group.Use(middleware.RequirePlatformTenant())
	group.GET(
		"/:id/storage-usage",
		permissionmiddleware.Require(checker, "platform.organization.manage"),
		h.Usage,
	)
}

func (h *PlatformAssetHandler) Usage(c *gin.Context) {
	scope, err := service.ScopeForOrganization(c.Param("id"))
	if err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "invalid organization id", http.StatusBadRequest))
		return
	}

	usage, err := h.assetService.GetUsage(c.Request.Context(), scope)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", usage)
}
