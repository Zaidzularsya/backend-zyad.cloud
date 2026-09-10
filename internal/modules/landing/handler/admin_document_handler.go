package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/dto"
	"zyad.cloud/internal/modules/landing/service"
)

// AdminDocumentHandler exposes the GrapesJS working copy of a landing page.
type AdminDocumentHandler struct {
	documentSvc service.DocumentService
}

func NewAdminDocumentHandler(documentSvc service.DocumentService) *AdminDocumentHandler {
	return &AdminDocumentHandler{documentSvc: documentSvc}
}

func (h *AdminDocumentHandler) RegisterRoutes(router *gin.RouterGroup, checker permissionmiddleware.CombinedPermissionChecker) {
	group := router.Group("/admin/landing-pages/:id/document")
	group.GET("", permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.page.read"), h.GetDocument)
	group.PUT("", permissionmiddleware.RequireOrganizationOrGlobal(checker, "landing.section.manage"), h.SaveDocument)
}

func (h *AdminDocumentHandler) GetDocument(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	doc, err := h.documentSvc.Get(c.Request.Context(), scope, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Landing document retrieved successfully", mapDocumentResponse(doc))
}

func (h *AdminDocumentHandler) SaveDocument(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	var req dto.SaveDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	doc, err := h.documentSvc.Save(c.Request.Context(), scope, service.SaveDocumentParams{
		PageID:  c.Param("id"),
		Project: req.Project,
		HTML:    req.HTML,
		CSS:     req.CSS,
		ActorID: permissionmiddleware.UserID(c),
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Landing document saved successfully", mapDocumentResponse(doc))
}

func mapDocumentResponse(doc domain.LandingPageDocument) dto.LandingDocumentResponse {
	project := doc.Project
	if project == nil {
		project = map[string]any{}
	}
	return dto.LandingDocumentResponse{
		LandingPageID: doc.LandingPageID,
		Project:       project,
		HTML:          doc.HTML,
		CSS:           doc.CSS,
		UpdatedAt:     doc.UpdatedAt,
	}
}
