package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/dto"
	"zyad.cloud/internal/modules/crm/service"
)

type ContactAttachmentHandler struct {
	svc service.ContactAttachmentService
}

func NewContactAttachmentHandler(svc service.ContactAttachmentService) *ContactAttachmentHandler {
	return &ContactAttachmentHandler{svc: svc}
}

// RegisterRoutes: lampiran adalah bagian dari data contact, jadi memakai
// permission contact.read/contact.update yang sudah ada, bukan permission baru.
func (h *ContactAttachmentHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/contacts/:id/attachments")

	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(p, "contact.read"), h.List)
	group.POST("", permissionmiddleware.RequireOrganizationOrGlobal(p, "contact.update"), h.Upload)
	group.GET("/:attachmentId/download", permissionmiddleware.RequireOrganizationOrGlobal(p, "contact.read"), h.Download)
	group.DELETE("/:attachmentId", permissionmiddleware.RequireOrganizationOrGlobal(p, "contact.update"), h.Delete)
}

func (h *ContactAttachmentHandler) List(c *gin.Context) {
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

	corehttp.OK(c, "success", dto.ContactAttachmentListFromDomain(attachments))
}

func (h *ContactAttachmentHandler) Upload(c *gin.Context) {
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

	attachment, err := h.svc.Upload(c.Request.Context(), scope, c.Param("id"), service.UploadContactAttachmentParams{
		Filename:   fileHeader.Filename,
		MimeType:   fileHeader.Header.Get("Content-Type"),
		SizeBytes:  fileHeader.Size,
		UploadedBy: userID,
	}, file)
	if err != nil {
		failAttachmentError(c, err)
		return
	}

	corehttp.OK(c, "success", dto.ContactAttachmentFromDomain(attachment))
}

func (h *ContactAttachmentHandler) Download(c *gin.Context) {
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

func (h *ContactAttachmentHandler) Delete(c *gin.Context) {
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
