package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/dto"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
	"zyad.cloud/internal/platform/storage"
)

type AdminMediaHandler struct {
	mediaService service.MediaService
}

func NewAdminMediaHandler(mediaService service.MediaService) *AdminMediaHandler {
	return &AdminMediaHandler{
		mediaService: mediaService,
	}
}

func (h *AdminMediaHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/admin/landing/media")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.media.manage"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.media.manage"), h.Upload)
	group.POST("/presign-upload", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.media.manage"), h.PresignUpload)
	group.POST("/confirm-upload", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.media.manage"), h.ConfirmUpload)
	group.POST("/presign-download", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.media.manage"), h.PresignDownload)
	// DELETE mapping based on the OpenAPI docs
	group.DELETE("/:id", permissionmiddleware.RequireOrganizationOrGlobal(p, "landing.media.manage"), h.Delete)
}

func (h *AdminMediaHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	assets, err := h.mediaService.ListAssets(c.Request.Context(), scope)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", assets)
}

func (h *AdminMediaHandler) Upload(c *gin.Context) {
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

	params := repository.CreateMediaAssetParams{
		Filename:  fileHeader.Filename,
		MimeType:  fileHeader.Header.Get("Content-Type"),
		SizeBytes: fileHeader.Size,
		CreatedBy: userID,
	}

	// AltText can be provided via form
	if altText := c.PostForm("alt_text"); altText != "" {
		params.AltText = altText
	}

	file, err := fileHeader.Open()
	if err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "failed to read uploaded file", http.StatusBadRequest))
		return
	}
	defer file.Close()

	asset, err := h.mediaService.UploadAsset(c.Request.Context(), scope, params, file)
	if err != nil {
		if err == service.ErrInvalidMimeType || err == service.ErrFileTooLarge {
			corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusBadRequest))
			return
		}
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", asset)
}

func (h *AdminMediaHandler) PresignUpload(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var req dto.PresignMediaUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	result, err := h.mediaService.PresignUpload(c.Request.Context(), scope, service.PresignUploadParams{
		Filename:  req.Filename,
		MimeType:  req.MimeType,
		SizeBytes: req.SizeBytes,
	})
	if err != nil {
		failMediaStorageError(c, err)
		return
	}

	corehttp.OK(c, "success", result)
}

func (h *AdminMediaHandler) ConfirmUpload(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var req dto.ConfirmMediaUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	asset, err := h.mediaService.ConfirmUpload(c.Request.Context(), scope, service.ConfirmUploadParams{
		ObjectKey: req.ObjectKey,
		Filename:  req.Filename,
		MimeType:  req.MimeType,
		SizeBytes: req.SizeBytes,
		AltText:   req.AltText,
		CreatedBy: permissionmiddleware.UserID(c),
	})
	if err != nil {
		failMediaStorageError(c, err)
		return
	}

	corehttp.Created(c, "success", asset)
}

func (h *AdminMediaHandler) PresignDownload(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	var req dto.PresignMediaDownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	result, err := h.mediaService.PresignDownload(c.Request.Context(), scope, req.ObjectKey)
	if err != nil {
		failMediaStorageError(c, err)
		return
	}

	corehttp.OK(c, "success", result)
}

// failMediaStorageError memetakan error dari media storage service ke response
// HTTP yang sesuai.
func failMediaStorageError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, storage.ErrPresignUnsupported):
		corehttp.Fail(c, coreerrors.New("STORAGE_PRESIGN_UNSUPPORTED", "storage provider tidak mendukung presigned url", http.StatusConflict))
	case errors.Is(err, service.ErrInvalidMimeType), errors.Is(err, service.ErrFileTooLarge):
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusBadRequest))
	case errors.Is(err, service.ErrUploadNotFound):
		corehttp.Fail(c, coreerrors.New("UPLOAD_NOT_FOUND", err.Error(), http.StatusNotFound))
	case errors.Is(err, service.ErrStorageRequired):
		corehttp.Fail(c, coreerrors.New("STORAGE_NOT_CONFIGURED", err.Error(), http.StatusServiceUnavailable))
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

func (h *AdminMediaHandler) Delete(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	id := c.Param("id")

	if err := h.mediaService.DeleteAsset(c.Request.Context(), scope, id); err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "deleted", nil)
}
