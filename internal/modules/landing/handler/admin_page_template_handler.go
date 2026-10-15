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

// AdminPageTemplateHandler melayani galeri template halaman (tabel global).
// Baca: semua org dengan landing.page.update. Kelola: org platform + permission manage.
type AdminPageTemplateHandler struct {
	svc service.PageTemplateService
}

func NewAdminPageTemplateHandler(svc service.PageTemplateService) *AdminPageTemplateHandler {
	return &AdminPageTemplateHandler{svc: svc}
}

func (h *AdminPageTemplateHandler) RegisterRoutes(router *gin.RouterGroup, checker permissionmiddleware.CombinedPermissionChecker) {
	read := permissionmiddleware.RequireOrganizationOrGlobal(checker, domain.PermissionPageUpdate)
	manage := permissionmiddleware.RequireOrganizationOrGlobal(checker, domain.PermissionPageTemplateManage)

	group := router.Group("/admin/landing-templates")
	group.GET("", read, h.List)
	// "/all" didaftarkan sebelum "/:id"; gin membedakan keduanya (static > param).
	group.GET("/all", manage, h.ListAll)
	group.GET("/:id", read, h.Get)
	group.POST("", manage, h.Create)
	group.POST("/:id/refresh-from-page", manage, h.Refresh)
	group.POST("/:id/copy-to-page", manage, h.CopyToPage)
	group.PATCH("/:id", manage, h.Patch)
}

func (h *AdminPageTemplateHandler) viewer(c *gin.Context) (service.PageTemplateViewer, error) {
	tenantContext, err := coretenant.RequireContext(c.Request.Context())
	if err != nil {
		return service.PageTemplateViewer{}, err
	}
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		return service.PageTemplateViewer{}, err
	}
	return service.PageTemplateViewer{
		Scope:      scope,
		IsPlatform: tenantContext.OrganizationType() == coretenant.OrganizationTypePlatform,
		ActorID:    permissionmiddleware.UserID(c),
	}, nil
}

func (h *AdminPageTemplateHandler) List(c *gin.Context) {
	v, err := h.viewer(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	items, err := h.svc.ListPublished(c.Request.Context(), v)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	out := make([]dto.PageTemplateSummary, 0, len(items))
	for _, t := range items {
		out = append(out, mapPageTemplateSummary(t))
	}
	corehttp.OK(c, "Landing templates retrieved successfully", out)
}

func (h *AdminPageTemplateHandler) ListAll(c *gin.Context) {
	v, err := h.viewer(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	items, err := h.svc.ListAll(c.Request.Context(), v)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	out := make([]dto.PageTemplateAdminSummary, 0, len(items))
	for _, t := range items {
		out = append(out, mapPageTemplateAdminSummary(t))
	}
	corehttp.OK(c, "Landing templates retrieved successfully", out)
}

func (h *AdminPageTemplateHandler) Get(c *gin.Context) {
	v, err := h.viewer(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	t, err := h.svc.Get(c.Request.Context(), v, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "Landing template retrieved successfully", mapPageTemplateDetail(t))
}

func (h *AdminPageTemplateHandler) Create(c *gin.Context) {
	v, err := h.viewer(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	var req dto.CreatePageTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	t, err := h.svc.CreateFromPage(c.Request.Context(), v, service.CreatePageTemplateInput{
		PageID: req.PageID, Name: req.Name, Slug: req.Slug, Category: req.Category,
		Description: req.Description, SortOrder: req.SortOrder,
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "Landing template created successfully", mapPageTemplateDetail(t))
}

func (h *AdminPageTemplateHandler) Refresh(c *gin.Context) {
	v, err := h.viewer(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	t, err := h.svc.RefreshFromPage(c.Request.Context(), v, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "Landing template refreshed successfully", mapPageTemplateDetail(t))
}

func (h *AdminPageTemplateHandler) CopyToPage(c *gin.Context) {
	v, err := h.viewer(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	page, err := h.svc.CopyToPage(c.Request.Context(), v, c.Param("id"))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.Created(c, "Landing template copied to page successfully", dto.PageTemplateCopyResponse{PageID: page.ID, Slug: page.Slug})
}

func (h *AdminPageTemplateHandler) Patch(c *gin.Context) {
	v, err := h.viewer(c)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	var req dto.UpdatePageTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	in := service.UpdatePageTemplateMetaInput{
		Name: req.Name, Description: req.Description, Category: req.Category,
		SortOrder: req.SortOrder, PlatformOnly: req.PlatformOnly,
	}
	if req.Status != nil {
		status := domain.PageTemplateStatus(*req.Status)
		in.Status = &status
	}
	t, err := h.svc.UpdateMeta(c.Request.Context(), v, c.Param("id"), in)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}
	corehttp.OK(c, "Landing template updated successfully", mapPageTemplateDetail(t))
}

func mapPageTemplateSummary(t domain.PageTemplate) dto.PageTemplateSummary {
	return dto.PageTemplateSummary{
		ID: t.ID, Slug: t.Slug, Name: t.Name, Description: t.Description, Category: t.Category,
		SortOrder: t.SortOrder, PlatformOnly: t.PlatformOnly, UpdatedAt: t.UpdatedAt,
	}
}

func mapPageTemplateAdminSummary(t domain.PageTemplate) dto.PageTemplateAdminSummary {
	return dto.PageTemplateAdminSummary{
		PageTemplateSummary: mapPageTemplateSummary(t),
		Status:              string(t.Status), Source: string(t.Source),
		SeedModified: t.SeedModified, SourcePageID: t.SourcePageID,
	}
}

func mapPageTemplateDetail(t domain.PageTemplate) dto.PageTemplateDetail {
	return dto.PageTemplateDetail{PageTemplateSummary: mapPageTemplateSummary(t), HTML: t.HTML, CSS: t.CSS}
}
