package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/asset/domain"
	"zyad.cloud/internal/modules/asset/service"
	"zyad.cloud/internal/platform/storage"
)

type AdminAssetHandler struct {
	assetService service.AssetService
}

func NewAdminAssetHandler(assetService service.AssetService) *AdminAssetHandler {
	return &AdminAssetHandler{assetService: assetService}
}

func (h *AdminAssetHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/admin/storage/objects")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "storage.object.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "storage.object.manage"), h.Upload)
	group.GET("/usage", permissionmiddleware.RequireOrganizationOrGlobal(p, "storage.object.read"), h.Usage)
	group.GET("/:id/download", permissionmiddleware.RequireOrganizationOrGlobal(p, "storage.object.read"), h.Download)
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "storage.object.manage"), h.Delete)
}

func (h *AdminAssetHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	objects, err := h.assetService.ListObjects(c.Request.Context(), scope)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", objects)
}

func (h *AdminAssetHandler) Upload(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "file is required", http.StatusBadRequest))
		return
	}

	class := domain.ObjectClass(c.PostForm("class"))
	if class == "" {
		class = domain.ObjectClassPrivate
	}

	params := service.UploadObjectParams{
		Filename:  fileHeader.Filename,
		MimeType:  fileHeader.Header.Get("Content-Type"),
		SizeBytes: fileHeader.Size,
		Class:     class,
		Label:     c.PostForm("label"),
		CreatedBy: userID,
	}

	file, err := fileHeader.Open()
	if err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "failed to read uploaded file", http.StatusBadRequest))
		return
	}
	defer file.Close()

	object, err := h.assetService.UploadObject(c.Request.Context(), scope, params, file)
	if err != nil {
		failAssetStorageError(c, err)
		return
	}

	corehttp.OK(c, "success", object)
}

func (h *AdminAssetHandler) Usage(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	usage, err := h.assetService.GetUsage(c.Request.Context(), scope)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", usage)
}

func (h *AdminAssetHandler) Download(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	result, err := h.assetService.DownloadURL(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		failAssetStorageError(c, err)
		return
	}

	corehttp.OK(c, "success", result)
}

func (h *AdminAssetHandler) Delete(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	if err := h.assetService.DeleteObject(c.Request.Context(), scope, c.Param("id")); err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "deleted", nil)
}

func failAssetStorageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrQuotaExceeded):
		corehttp.Fail(c, coreerrors.New("STORAGE_QUOTA_EXCEEDED", "kuota storage tenant sudah penuh", http.StatusConflict))
	case errors.Is(err, service.ErrInvalidMimeType), errors.Is(err, service.ErrFileTooLarge):
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusBadRequest))
	case errors.Is(err, service.ErrStorageRequired):
		corehttp.Fail(c, coreerrors.New("STORAGE_NOT_CONFIGURED", err.Error(), http.StatusServiceUnavailable))
	case errors.Is(err, storage.ErrPresignUnsupported):
		corehttp.Fail(c, coreerrors.New("STORAGE_PRESIGN_UNSUPPORTED", "storage provider tidak mendukung presigned url", http.StatusConflict))
	case errors.Is(err, storage.ErrInvalidTenantObject):
		corehttp.Fail(c, coreerrors.New("STORAGE_OBJECT_OWNERSHIP_INVALID", "object bukan milik organization", http.StatusForbidden))
	default:
		var appErr *coreerrors.AppError
		if errors.As(err, &appErr) {
			corehttp.Fail(c, appErr)
			return
		}
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
	}
}
