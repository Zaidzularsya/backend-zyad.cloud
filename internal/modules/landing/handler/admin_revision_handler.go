package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/service"
)

type AdminRevisionHandler struct {
	revisionService service.RevisionService
}

func NewAdminRevisionHandler(revisionService service.RevisionService) *AdminRevisionHandler {
	return &AdminRevisionHandler{
		revisionService: revisionService,
	}
}

func (h *AdminRevisionHandler) RegisterRoutes(router *gin.RouterGroup, p permissionmiddleware.PermissionChecker) {
	group := router.Group("/admin/landing-pages/:id/revisions")

	group.GET("/compare", permissionmiddleware.Require(p, "landing.page.read"), h.Compare)
}

func (h *AdminRevisionHandler) List(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	pageID := c.Param("id")

	revisions, err := h.revisionService.ListRevisions(c.Request.Context(), scope, pageID)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", revisions)
}

func (h *AdminRevisionHandler) Compare(c *gin.Context) {
	// For compare, usually we just return the two requested revisions or diff
	// MVP: return Not Implemented or just fetch both revisions manually
	corehttp.Fail(c, coreerrors.New("NOT_IMPLEMENTED", "compare is not yet implemented natively", http.StatusNotImplemented))
}

func (h *AdminRevisionHandler) Restore(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, coreerrors.New("UNAUTHORIZED", "missing scope", http.StatusUnauthorized))
		return
	}

	userID := permissionmiddleware.UserID(c)

	pageID := c.Param("id") // Ensure scope matching
	_ = pageID

	revisionID := c.Param("revision")

	page, err := h.revisionService.RestoreRevision(c.Request.Context(), scope, revisionID, userID)
	if err != nil {
		corehttp.Fail(c, coreerrors.New("INTERNAL_ERROR", err.Error(), http.StatusInternalServerError))
		return
	}

	corehttp.OK(c, "success", page)
}
