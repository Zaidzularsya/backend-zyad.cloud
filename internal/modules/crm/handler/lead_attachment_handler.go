package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	assetservice "zyad.cloud/internal/modules/asset/service"
	"zyad.cloud/internal/modules/crm/dto"
	"zyad.cloud/internal/modules/crm/service"
	"zyad.cloud/internal/platform/storage"
)

type LeadAttachmentHandler struct {
	svc service.LeadAttachmentService
}

func NewLeadAttachmentHandler(svc service.LeadAttachmentService) *LeadAttachmentHandler {
	return &LeadAttachmentHandler{svc: svc}
}

// RegisterRoutes: lampiran adalah bagian dari data lead, jadi memakai
// permission lead.read/lead.update yang sudah ada, bukan permission baru.
func (h *LeadAttachmentHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/leads/:id/attachments")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.update"), h.Upload)
	group.GET("/:attachmentId/download", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.read"), h.Download)
	group.DELETE("/:attachmentId", permissionmiddleware.RequireOrganizationOrGlobal(p, "lead.update"), h.Delete)
}

func (h *LeadAttachmentHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	attachments, err := h.svc.List(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		failAttachmentError(c, err)
		return
	}

	corehttp.OK(c, "success", dto.LeadAttachmentListFromDomain(attachments))
}

func (h *LeadAttachmentHandler) Upload(c *gin.Context) {
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

	file, err := fileHeader.Open()
	if err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "failed to read uploaded file", http.StatusBadRequest))
		return
	}
	defer file.Close()

	attachment, err := h.svc.Upload(c.Request.Context(), scope, c.Param("id"), service.UploadLeadAttachmentParams{
		Filename:   fileHeader.Filename,
		MimeType:   fileHeader.Header.Get("Content-Type"),
		SizeBytes:  fileHeader.Size,
		UploadedBy: userID,
	}, file)
	if err != nil {
		failAttachmentError(c, err)
		return
	}

	corehttp.OK(c, "success", dto.LeadAttachmentFromDomain(attachment))
}

func (h *LeadAttachmentHandler) Download(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	result, err := h.svc.DownloadURL(c.Request.Context(), scope, c.Param("id"), c.Param("attachmentId"))
	if err != nil {
		failAttachmentError(c, err)
		return
	}

	corehttp.OK(c, "success", result)
}

func (h *LeadAttachmentHandler) Delete(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	if err := h.svc.Delete(c.Request.Context(), scope, c.Param("id"), c.Param("attachmentId")); err != nil {
		failAttachmentError(c, err)
		return
	}

	corehttp.OK(c, "deleted", nil)
}

// failAttachmentError memetakan error modul asset (upload/presign) sama
// seperti AdminAssetHandler, supaya pesan ke FE konsisten di kedua endpoint.
func failAttachmentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, assetservice.ErrQuotaExceeded):
		corehttp.Fail(c, coreerrors.New("STORAGE_QUOTA_EXCEEDED", "kuota storage tenant sudah penuh", http.StatusConflict))
	case errors.Is(err, assetservice.ErrInvalidMimeType):
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "tipe file tidak didukung (hanya JPG, PNG, WEBP, PDF)", http.StatusBadRequest))
	case errors.Is(err, assetservice.ErrFileTooLarge):
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", "ukuran file maksimal 10MB", http.StatusBadRequest))
	case errors.Is(err, assetservice.ErrStorageRequired):
		corehttp.Fail(c, coreerrors.New("STORAGE_NOT_CONFIGURED", err.Error(), http.StatusServiceUnavailable))
	case errors.Is(err, storage.ErrPresignUnsupported):
		corehttp.Fail(c, coreerrors.New("STORAGE_PRESIGN_UNSUPPORTED", "storage provider tidak mendukung presigned url", http.StatusConflict))
	default:
		corehttp.Fail(c, err)
	}
}
