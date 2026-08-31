package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
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
