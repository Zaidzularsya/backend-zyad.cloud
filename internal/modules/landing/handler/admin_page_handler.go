package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	coreerrors "zyad.cloud/internal/core/errors"
	corehttp "zyad.cloud/internal/core/http"
	"zyad.cloud/internal/core/middleware"
	permissionmiddleware "zyad.cloud/internal/core/permission/middleware"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/dto"
	"zyad.cloud/internal/modules/landing/repository"
	"zyad.cloud/internal/modules/landing/service"
	"zyad.cloud/internal/shared/response"
)

type AdminPageHandler struct {
	pageSvc       service.PageService
	visibilitySvc service.VisibilityService
	revisionSvc   service.RevisionService
	publishSvc    service.PublishService
}

func NewAdminPageHandler(
	pageSvc service.PageService,
	visibilitySvc service.VisibilityService,
	revisionSvc service.RevisionService,
	publishSvc service.PublishService,
) *AdminPageHandler {
	return &AdminPageHandler{
		pageSvc:       pageSvc,
		visibilitySvc: visibilitySvc,
		revisionSvc:   revisionSvc,
		publishSvc:    publishSvc,
	}
}

func (h *AdminPageHandler) RegisterRoutes(router *gin.RouterGroup, checker permissionmiddleware.PermissionChecker) {
	group := router.Group("/admin/landing-pages")
	group.Use(middleware.RequireActiveTenant())

	group.GET("", permissionmiddleware.Require(checker, "landing.page.read"), h.ListPages)
	group.POST("", permissionmiddleware.Require(checker, "landing.page.create"), h.CreatePage)
	group.POST("/from-template", permissionmiddleware.Require(checker, "landing.page.create"), h.CreatePageFromTemplate)
	group.GET("/:id", permissionmiddleware.Require(checker, "landing.page.read"), h.GetPage)
	group.PATCH("/:id", permissionmiddleware.Require(checker, "landing.page.update"), h.UpdatePage)
	group.DELETE("/:id", permissionmiddleware.Require(checker, "landing.page.delete"), h.DeletePage)
	group.PUT("/:id/access", permissionmiddleware.Require(checker, "landing.page.update"), h.UpdatePageAccess)
	group.POST("/:id/duplicate", permissionmiddleware.Require(checker, "landing.page.create"), h.DuplicatePage)
	group.POST("/:id/archive", permissionmiddleware.Require(checker, "landing.page.archive"), h.ArchivePage)
	group.POST("/:id/restore", permissionmiddleware.Require(checker, "landing.page.restore"), h.RestorePage)
	group.PATCH("/:id/seo", permissionmiddleware.Require(checker, "landing.seo.manage"), h.UpdatePageSEO)
	group.GET("/:id/revisions", permissionmiddleware.Require(checker, "landing.page.read"), h.ListRevisions)
	group.POST("/:id/revisions/:version/restore", permissionmiddleware.Require(checker, "landing.page.publish"), h.RestoreRevision)
	group.POST("/:id/publish", permissionmiddleware.Require(checker, "landing.page.publish"), h.PublishPage)
	group.POST("/:id/unpublish", permissionmiddleware.Require(checker, "landing.page.publish"), h.UnpublishPage)
	group.PUT("/:id/schedule", permissionmiddleware.Require(checker, "landing.page.publish"), h.SchedulePage)
	group.DELETE("/:id/schedule", permissionmiddleware.Require(checker, "landing.page.publish"), h.CancelSchedule)
}

// ListPages godoc
// @Summary List landing pages
// @Description Retrieve a list of landing pages for the active tenant
func (h *AdminPageHandler) ListPages(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	var query dto.PageListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}
	page := query.Page
	if page <= 0 {
		page = 1
	}
	perPage := query.PerPage
	if perPage <= 0 {
		perPage = 10
	}
	isTemplate := query.IsTemplate
	if isTemplate == nil {
		defaultIsTemplate := false
		isTemplate = &defaultIsTemplate
	}

	filter := repository.PageListFilter{
		Status:         domain.PageStatus(query.Status),
		PageType:       domain.PageType(query.PageType),
		IsTemplate:     isTemplate,
		IncludeDeleted: query.IncludeDeleted,
		Limit:          perPage,
		Offset:         (page - 1) * perPage,
	}

	pages, total, err := h.pageSvc.List(c.Request.Context(), scope, filter)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	var dtos []dto.PageResponse
	for _, p := range pages {
		dtos = append(dtos, pageResponseFromDomain(p))
	}

	totalPages := 0
	if perPage > 0 {
		totalPages = (int(total) + perPage - 1) / perPage
	}
	payload := dto.PageListResponse{
		Items: dtos,
		Meta: dto.PaginationMeta{
			Total:      total,
			Page:       page,
			PerPage:    perPage,
			TotalPages: totalPages,
		},
	}

	response.JSON(c, http.StatusOK, "Pages retrieved successfully", payload, nil)
}

// CreatePage godoc
// @Summary Create a new landing page
func (h *AdminPageHandler) CreatePage(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	var req dto.CreatePageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	params := repository.CreatePageParams{
		Name:       req.Name,
		Title:      req.Title,
		Slug:       req.Slug,
		Type:       domain.PageType(req.PageType),
		Status:     domain.PageStatusDraft,
		Visibility: domain.PageVisibility(req.Visibility),
		Locale:     req.Locale,
		Timezone:   req.Timezone,
		IsHomepage: req.IsHomepage,
		IsTemplate: req.IsTemplate,
		CreatedBy:  permissionmiddleware.UserID(c),
	}
	if req.Settings != nil {
		settings := pageSettingsToDomain(*req.Settings)
		params.Settings = &settings
	}

	page, err := h.pageSvc.Create(c.Request.Context(), scope, params)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusCreated, "Page created successfully", pageResponseFromDomain(page), nil)
}

// CreatePageFromTemplate godoc
// @Summary Create a new landing page from a page-level template
func (h *AdminPageHandler) CreatePageFromTemplate(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	var req dto.CreatePageFromTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	page, err := h.pageSvc.InstantiateFromTemplate(c.Request.Context(), scope, service.InstantiatePageFromTemplateParams{
		TemplatePageID:  req.TemplatePageID,
		Name:            req.Name,
		Title:           req.Title,
		Slug:            req.Slug,
		Visibility:      domain.PageVisibility(req.Visibility),
		Locale:          req.Locale,
		Timezone:        req.Timezone,
		IncludeBranding: req.IncludeBranding,
		CreatedBy:       permissionmiddleware.UserID(c),
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusCreated, "Page created from template successfully", pageResponseFromDomain(page), nil)
}

// GetPage godoc
// @Summary Get a landing page by ID
func (h *AdminPageHandler) GetPage(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	page, err := h.pageSvc.Get(c.Request.Context(), scope, pageID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Page retrieved successfully", pageResponseFromDomain(page))
}

func pageSettingsFromDomain(settings domain.PageSettings) dto.PageSettingsResponse {
	badges := make([]dto.TrustBadgeItem, 0, len(settings.TrustBadges))
	for _, b := range settings.TrustBadges {
		badges = append(badges, dto.TrustBadgeItem{ImageURL: b.ImageURL, Label: b.Label})
	}
	return dto.PageSettingsResponse{
		PublishRequireApproval: settings.PublishRequireApproval,
		LeadNotificationEmails: settings.LeadNotificationEmails,
		FooterCopyrightText:    settings.FooterCopyrightText,
		TrustBadges:            badges,
		AnalyticsHooks:         settings.AnalyticsHooks,
	}
}

func pageSettingsToDomain(req dto.PageSettingsRequest) domain.PageSettings {
	badges := make([]domain.TrustBadge, 0, len(req.TrustBadges))
	for _, b := range req.TrustBadges {
		badges = append(badges, domain.TrustBadge{ImageURL: b.ImageURL, Label: b.Label})
	}
	return domain.PageSettings{
		PublishRequireApproval: req.PublishRequireApproval,
		LeadNotificationEmails: req.LeadNotificationEmails,
		FooterCopyrightText:    req.FooterCopyrightText,
		TrustBadges:            badges,
		AnalyticsHooks:         req.AnalyticsHooks,
	}
}

func pageResponseFromDomain(page domain.LandingPage) dto.PageResponse {
	return dto.PageResponse{
		ID:               page.ID,
		Name:             page.Name,
		Title:            page.Title,
		Slug:             page.Slug,
		PageType:         string(page.Type),
		Status:           string(page.Status),
		Visibility:       string(page.Visibility),
		Locale:           page.Locale,
		Timezone:         page.Timezone,
		IsHomepage:       page.IsHomepage,
		IsTemplate:       page.IsTemplate,
		PublishedVersion: page.PublishedVersion,
		PublishAt:        page.PublishAt,
		UnpublishAt:      page.UnpublishAt,
		PublishedAt:      page.PublishedAt,
		Settings:         pageSettingsFromDomain(page.Settings),
		SEO:              page.SEO,
		CreatedAt:        page.CreatedAt,
		UpdatedAt:        page.UpdatedAt,
		DeletedAt:        page.DeletedAt,
	}
}

// UpdatePage godoc
// @Summary Update a landing page
func (h *AdminPageHandler) UpdatePage(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	var req dto.UpdatePageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	pageID := c.Param("id")

	var pageType *domain.PageType
	if req.PageType != nil {
		pt := domain.PageType(*req.PageType)
		pageType = &pt
	}

	var visibility *domain.PageVisibility
	if req.Visibility != nil {
		v := domain.PageVisibility(*req.Visibility)
		visibility = &v
	}

	params := repository.UpdatePageParams{
		Name:       req.Name,
		Title:      req.Title,
		Slug:       req.Slug,
		Type:       pageType,
		Visibility: visibility,
		Locale:     req.Locale,
		Timezone:   req.Timezone,
		IsHomepage: req.IsHomepage,
		IsTemplate: req.IsTemplate,
		UpdatedBy:  permissionmiddleware.UserID(c),
	}
	if req.Settings != nil {
		settings := pageSettingsToDomain(*req.Settings)
		params.Settings = &settings
	}

	page, err := h.pageSvc.Update(c.Request.Context(), scope, pageID, params)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Page updated successfully", page)
}

// DeletePage godoc
func (h *AdminPageHandler) DeletePage(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	if err := h.pageSvc.Delete(c.Request.Context(), scope, pageID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Page deleted successfully", nil)
}

// UpdatePageAccess godoc
func (h *AdminPageHandler) UpdatePageAccess(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	var req dto.UpdatePageAccessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	pageID := c.Param("id")
	userID := permissionmiddleware.UserID(c)
	visibility := domain.PageVisibility(req.Visibility)

	// Update visibility first
	if err := h.visibilitySvc.UpdateVisibility(c.Request.Context(), scope, pageID, visibility, userID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	if visibility == domain.PageVisibilityPasswordProtected && req.Password != "" {
		if err := h.visibilitySvc.SetPassword(c.Request.Context(), scope, pageID, req.Password, userID); err != nil {
			corehttp.Fail(c, err)
			return
		}
	} else if visibility != domain.PageVisibilityPasswordProtected {
		if err := h.visibilitySvc.RemovePassword(c.Request.Context(), scope, pageID, userID); err != nil {
			corehttp.Fail(c, err)
			return
		}
	}

	corehttp.OK(c, "Page access updated successfully", nil)
}

// DuplicatePage godoc
func (h *AdminPageHandler) DuplicatePage(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")

	var req dto.DuplicatePageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	// For MVP duplicate page name/slug is managed inside Service, but service interface duplicate doesn't take name/slug
	// Looking at PageService: Duplicate(ctx, scope, DuplicatePageParams)
	page, err := h.pageSvc.Duplicate(c.Request.Context(), scope, service.DuplicatePageParams{
		PageID: pageID,
		UserID: permissionmiddleware.UserID(c),
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	// Then update the name and slug
	page, err = h.pageSvc.Update(c.Request.Context(), scope, page.ID, repository.UpdatePageParams{
		Name:      &req.Name,
		Slug:      &req.Slug,
		UpdatedBy: permissionmiddleware.UserID(c),
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	response.JSON(c, http.StatusCreated, "Page duplicated successfully", page, nil)
}

// ArchivePage godoc
func (h *AdminPageHandler) ArchivePage(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	userID := permissionmiddleware.UserID(c)

	if err := h.pageSvc.Archive(c.Request.Context(), scope, pageID, userID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Page archived successfully", nil)
}

// RestorePage godoc
func (h *AdminPageHandler) RestorePage(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	userID := permissionmiddleware.UserID(c)

	if err := h.pageSvc.Restore(c.Request.Context(), scope, pageID, userID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Page restored successfully", nil)
}

// UpdatePageSEO godoc
func (h *AdminPageHandler) UpdatePageSEO(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	var req dto.UpdatePageSEORequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	// Just reuse UpdatePage for SEO fields for now since PageService.Update SEO is merged in UpdatePageParams
	// Actually, UpdatePageParams doesn't have SEO directly. It might be in Settings? No, `SEO map[string]any` is in Create/UpdatePageParams in repository.
	// We'll construct a map from the req struct.
	seoMap := map[string]any{
		"meta_title":       req.MetaTitle,
		"meta_description": req.MetaDescription,
		"meta_keywords":    req.MetaKeywords,
		"canonical_url":    req.CanonicalURL,
		"robots":           req.Robots,
		"open_graph":       req.OpenGraph,
		"twitter_card":     req.TwitterCard,
		"schema_markup":    req.SchemaMarkup,
		"sitemap":          req.Sitemap,
	}

	page, err := h.pageSvc.Update(c.Request.Context(), scope, pageID, repository.UpdatePageParams{
		// Map Settings payload if your service handles it. Assuming UpdatePageParams accepts it.
		// Wait, repository.UpdatePageParams in page_contract.go has SEO map[string]any. Let's use it.
		SEO:       seoMap,
		UpdatedBy: permissionmiddleware.UserID(c),
	})
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Page SEO updated successfully", page)
}

// PublishPage godoc
func (h *AdminPageHandler) PublishPage(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	userID := permissionmiddleware.UserID(c)

	// Since we don't have change_note in payload per contract (or maybe we do), just pass empty for now
	version, err := h.publishSvc.Publish(c.Request.Context(), scope, pageID, "Published from API", userID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Page published successfully", version)
}

// UnpublishPage godoc
func (h *AdminPageHandler) UnpublishPage(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	userID := permissionmiddleware.UserID(c)

	if err := h.publishSvc.Unpublish(c.Request.Context(), scope, pageID, userID); err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Page unpublished successfully", nil)
}

// ListRevisions godoc
func (h *AdminPageHandler) ListRevisions(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	revisions, err := h.revisionSvc.ListRevisions(c.Request.Context(), scope, pageID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Revisions retrieved successfully", revisions)
}

// RestoreRevision godoc
func (h *AdminPageHandler) RestoreRevision(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	version := c.Param("version")
	page, err := h.publishSvc.RestoreVersion(c.Request.Context(), scope, c.Param("id"), version, permissionmiddleware.UserID(c))
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	corehttp.OK(c, "Revision restored successfully", page)
}

// SchedulePage godoc
func (h *AdminPageHandler) SchedulePage(c *gin.Context) {
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	var req dto.ScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		corehttp.Fail(c, coreerrors.New("VALIDATION_ERROR", err.Error(), http.StatusUnprocessableEntity))
		return
	}

	if req.PublishAt != nil {
		_, err = h.revisionSvc.ScheduleAction(c.Request.Context(), scope, service.SchedulePublishParams{
			PageID:      c.Param("id"),
			Action:      domain.ScheduleActionPublish,
			ScheduledAt: *req.PublishAt,
			ActorID:     permissionmiddleware.UserID(c),
		})
		if err != nil {
			corehttp.Fail(c, err)
			return
		}
	}

	if req.UnpublishAt != nil {
		_, err = h.revisionSvc.ScheduleAction(c.Request.Context(), scope, service.SchedulePublishParams{
			PageID:      c.Param("id"),
			Action:      domain.ScheduleActionUnpublish,
			ScheduledAt: *req.UnpublishAt,
			ActorID:     permissionmiddleware.UserID(c),
		})
		if err != nil {
			corehttp.Fail(c, err)
			return
		}
	}

	corehttp.OK(c, "Page schedule updated successfully", nil)
}

// CancelSchedule godoc
func (h *AdminPageHandler) CancelSchedule(c *gin.Context) {
	// Not implemented fully yet because we need scheduleID, but the contract says DELETE /:id/schedule
	// It deletes all schedules for the page or requires scheduleId in body/query. We will just cancel all.
	// Actually, the contract has DELETE /:id/schedule without schedule ID. Let's list and delete them.
	scope, err := coretenant.RequireScope(c.Request.Context())
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	pageID := c.Param("id")
	schedules, err := h.revisionSvc.ListSchedules(c.Request.Context(), scope, pageID)
	if err != nil {
		corehttp.Fail(c, err)
		return
	}

	for _, s := range schedules {
		if s.Status == domain.ScheduleStatusPending {
			if err := h.revisionSvc.CancelSchedule(c.Request.Context(), scope, s.ID); err != nil {
				corehttp.Fail(c, err)
				return
			}
		}
	}

	corehttp.OK(c, "Page schedule cancelled successfully", nil)
}
