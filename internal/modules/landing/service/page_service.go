package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type pageService struct {
	pageRepo     repository.PageRepository
	sectionRepo  repository.SectionRepository
	brandingRepo repository.BrandingRepository
	quotaGuard   LandingPageQuotaGuard
}

type LandingPageQuotaGuard interface {
	RequireQuotaValue(
		context.Context,
		string,
		string,
		string,
		int64,
		int64,
	) error
}

type PageServiceOption func(*pageService)

func WithLandingPageQuotaGuard(guard LandingPageQuotaGuard) PageServiceOption {
	return func(service *pageService) {
		service.quotaGuard = guard
	}
}

// WithLandingPageBrandingRepo enables copying a source page's per-page branding
// override when duplicating a page or instantiating one from a template. It is
// optional: without it, branding copy is simply skipped.
func WithLandingPageBrandingRepo(brandingRepo repository.BrandingRepository) PageServiceOption {
	return func(service *pageService) {
		service.brandingRepo = brandingRepo
	}
}

func NewPageService(pageRepo repository.PageRepository, sectionRepo repository.SectionRepository, options ...PageServiceOption) PageService {
	service := &pageService{
		pageRepo:    pageRepo,
		sectionRepo: sectionRepo,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *pageService) Create(ctx context.Context, scope coretenant.Scope, params repository.CreatePageParams) (domain.LandingPage, error) {
	if !params.IsTemplate {
		if err := s.requireCreateQuota(ctx, scope); err != nil {
			return domain.LandingPage{}, err
		}
	}
	// Sanitize and generate a safe slug
	params.Slug = generateSafeSlug(params.Slug, params.Title)

	page, err := s.pageRepo.Create(ctx, scope, params)
	if err != nil {
		return domain.LandingPage{}, mapPagePersistenceError(err)
	}

	// Every real (non-template) section-builder page starts with a sticky Header
	// section at the very top. Its link items are synthesized at render time from
	// the tenant's location=header menu (see the frontend renderer); deleting this
	// section leaves the page without a menu. Seeded via the repo so it does not
	// count against the section quota. Non-fatal: page create still succeeds if
	// this fails, the header can be re-added from the builder palette. GrapesJS
	// pages have no sections, so they are not seeded.
	if !params.IsTemplate && params.Builder != domain.PageBuilderGrapesJS {
		_, _ = s.sectionRepo.Create(ctx, scope, repository.CreateSectionParams{
			LandingPageID: page.ID,
			Key:           "header",
			Type:          domain.SectionTypeHeader,
			Name:          "Header",
			SortOrder:     0,
			IsEnabled:     true,
			Content:       map[string]any{"sticky": true, "showLoginCta": true},
			Style:         map[string]any{},
			CreatedBy:     params.CreatedBy,
		})
	}

	return page, nil
}

func (s *pageService) requireCreateQuota(ctx context.Context, scope coretenant.Scope) error {
	if s.quotaGuard == nil {
		return nil
	}
	isTemplate := false
	_, total, err := s.pageRepo.List(ctx, scope, repository.PageListFilter{
		IsTemplate: &isTemplate,
		Limit:      1,
	})
	if err != nil {
		return mapPagePersistenceError(err)
	}
	return s.quotaGuard.RequireQuotaValue(
		ctx,
		scope.OrganizationID(),
		domain.FeatureLandingMaxPages,
		"limit",
		total,
		1,
	)
}

func (s *pageService) Get(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingPage, error) {
	return s.pageRepo.FindByID(ctx, scope, id)
}

func (s *pageService) List(ctx context.Context, scope coretenant.Scope, filter repository.PageListFilter) ([]domain.LandingPage, int64, error) {
	return s.pageRepo.List(ctx, scope, filter)
}

func (s *pageService) Update(ctx context.Context, scope coretenant.Scope, id string, params repository.UpdatePageParams) (domain.LandingPage, error) {
	if params.Slug != nil {
		safeSlug := generateSafeSlug(*params.Slug, "")
		params.Slug = &safeSlug
	}
	page, err := s.pageRepo.Update(ctx, scope, id, params)
	if err != nil {
		return domain.LandingPage{}, mapPagePersistenceError(err)
	}
	return page, nil
}

func (s *pageService) Delete(ctx context.Context, scope coretenant.Scope, id string) error {
	if err := s.pageRepo.Delete(ctx, scope, id); err != nil {
		return mapPagePersistenceError(err)
	}
	return nil
}

func (s *pageService) Duplicate(ctx context.Context, scope coretenant.Scope, params DuplicatePageParams) (domain.LandingPage, error) {
	// 1. Get original page
	originalPage, err := s.pageRepo.FindByID(ctx, scope, params.PageID)
	if err != nil {
		return domain.LandingPage{}, err
	}

	// 2. Load sections first so quota can be checked before creating the duplicate page.
	sections, err := s.sectionRepo.ListByPage(ctx, scope, originalPage.ID)
	if err != nil {
		return domain.LandingPage{}, err
	}
	if err := s.requireDuplicateSectionQuota(ctx, scope, len(sections)); err != nil {
		return domain.LandingPage{}, err
	}

	// 3. Generate new slug
	newSlug := fmt.Sprintf("%s-copy-%d", originalPage.Slug, time.Now().Unix())

	// 4. Create new page as Draft
	newPage, err := s.pageRepo.Create(ctx, scope, repository.CreatePageParams{
		Name:       originalPage.Name + " (Copy)",
		Title:      originalPage.Title,
		Slug:       newSlug,
		Type:       originalPage.Type,
		Status:     domain.PageStatusDraft,
		Visibility: originalPage.Visibility,
		Locale:     originalPage.Locale,
		Timezone:   originalPage.Timezone,
		IsHomepage: false,
		IsTemplate: false,
		CreatedBy:  params.UserID,
	})
	if err != nil {
		return domain.LandingPage{}, err
	}

	// 5. Copy sections, SEO, and branding override onto the duplicate.
	return s.copyPageComposition(ctx, scope, originalPage, newPage, sections, params.UserID, true)
}

// InstantiateFromTemplate creates a brand new page seeded from an existing
// page-level template (IsTemplate=true), copying its sections, SEO, and
// optionally its per-page branding override in one call.
func (s *pageService) InstantiateFromTemplate(
	ctx context.Context,
	scope coretenant.Scope,
	params InstantiatePageFromTemplateParams,
) (domain.LandingPage, error) {
	templatePage, err := s.pageRepo.FindByID(ctx, scope, params.TemplatePageID)
	if err != nil {
		return domain.LandingPage{}, err
	}
	if !templatePage.IsTemplate {
		return domain.LandingPage{}, coreerrors.New(
			"PAGE_NOT_TEMPLATE",
			"source page is not a template",
			http.StatusUnprocessableEntity,
		)
	}

	if err := s.requireCreateQuota(ctx, scope); err != nil {
		return domain.LandingPage{}, err
	}

	sections, err := s.sectionRepo.ListByPage(ctx, scope, templatePage.ID)
	if err != nil {
		return domain.LandingPage{}, err
	}
	if err := s.requireDuplicateSectionQuota(ctx, scope, len(sections)); err != nil {
		return domain.LandingPage{}, err
	}

	newPage, err := s.pageRepo.Create(ctx, scope, repository.CreatePageParams{
		Name:       params.Name,
		Title:      params.Title,
		Slug:       generateSafeSlug(params.Slug, params.Title),
		Type:       templatePage.Type,
		Status:     domain.PageStatusDraft,
		Visibility: params.Visibility,
		Locale:     params.Locale,
		Timezone:   params.Timezone,
		IsHomepage: false,
		IsTemplate: false,
		CreatedBy:  params.CreatedBy,
	})
	if err != nil {
		return domain.LandingPage{}, mapPagePersistenceError(err)
	}

	page, err := s.copyPageComposition(ctx, scope, templatePage, newPage, sections, params.CreatedBy, params.IncludeBranding)
	if err != nil {
		return domain.LandingPage{}, mapPagePersistenceError(err)
	}
	return page, nil
}

// copyPageComposition copies sections, SEO, and (optionally) a per-page
// branding override from sourcePage onto newPage. It is the shared primitive
// behind both Duplicate and InstantiateFromTemplate.
//
// No cross-repository transaction manager exists in this module yet, so this
// is sequential-calls-plus-defensive-rollback rather than a single DB
// transaction: if section copying fails partway through, the just-created
// page is deleted rather than left half-composed. SEO/branding copy failures
// are non-fatal (best-effort) since the page is already usable without them.
func (s *pageService) copyPageComposition(
	ctx context.Context,
	scope coretenant.Scope,
	sourcePage domain.LandingPage,
	newPage domain.LandingPage,
	sections []domain.LandingSection,
	userID string,
	includeBranding bool,
) (domain.LandingPage, error) {
	for _, sec := range sections {
		if _, err := s.sectionRepo.Create(ctx, scope, repository.CreateSectionParams{
			LandingPageID: newPage.ID,
			Key:           sec.Key,
			Type:          sec.Type,
			Name:          sec.Name,
			SortOrder:     sec.SortOrder,
			IsEnabled:     sec.IsEnabled,
			Content:       sec.Content,
			Style:         sec.Style,
			CreatedBy:     userID,
		}); err != nil {
			_ = s.pageRepo.Delete(ctx, scope, newPage.ID)
			return domain.LandingPage{}, err
		}
	}

	if len(sourcePage.SEO) > 0 {
		if updated, err := s.pageRepo.Update(ctx, scope, newPage.ID, repository.UpdatePageParams{
			SEO:       sourcePage.SEO,
			UpdatedBy: userID,
		}); err == nil {
			newPage = updated
		}
	}

	if includeBranding && s.brandingRepo != nil {
		if sourceBranding, err := s.brandingRepo.GetByPage(ctx, scope, sourcePage.ID); err == nil {
			_, _ = s.brandingRepo.Upsert(ctx, scope, repository.CreateBrandingParams{
				LandingPageID:  &newPage.ID,
				CompanyName:    &sourceBranding.CompanyName,
				Tagline:        &sourceBranding.Tagline,
				LogoLightURL:   &sourceBranding.LogoLightURL,
				LogoDarkURL:    &sourceBranding.LogoDarkURL,
				FaviconURL:     &sourceBranding.FaviconURL,
				SocialImageURL: &sourceBranding.SocialImageURL,
				Colors:         &sourceBranding.Colors,
				Typography:     &sourceBranding.Typography,
				Shape:          &sourceBranding.Shape,
				Layout:         &sourceBranding.Layout,
				Contact:        &sourceBranding.Contact,
				SocialLinks:    sourceBranding.SocialLinks,
			})
		}
	}

	return newPage, nil
}

func (s *pageService) requireDuplicateSectionQuota(
	ctx context.Context,
	scope coretenant.Scope,
	sectionCount int,
) error {
	if s.quotaGuard == nil || sectionCount <= 0 {
		return nil
	}
	return s.quotaGuard.RequireQuotaValue(
		ctx,
		scope.OrganizationID(),
		domain.FeatureLandingMaxSections,
		"limit",
		0,
		int64(sectionCount),
	)
}

func (s *pageService) Archive(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) error {
	status := domain.PageStatusArchived
	_, err := s.pageRepo.Update(ctx, scope, id, repository.UpdatePageParams{
		Status:    &status,
		UpdatedBy: updatedBy,
	})
	return mapPagePersistenceError(err)
}

func (s *pageService) Restore(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) error {
	status := domain.PageStatusDraft
	_, err := s.pageRepo.Update(ctx, scope, id, repository.UpdatePageParams{
		Status:    &status,
		UpdatedBy: updatedBy,
	})
	return mapPagePersistenceError(err)
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-z0-9]+`)

func generateSafeSlug(slug string, title string) string {
	target := slug
	if target == "" {
		target = title
	}
	target = strings.ToLower(strings.TrimSpace(target))
	target = nonAlphanumericRegex.ReplaceAllString(target, "-")
	target = strings.Trim(target, "-")
	if target == "" {
		target = fmt.Sprintf("page-%d", time.Now().Unix())
	}
	return target
}

func mapPagePersistenceError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return coreerrors.New(
			"PAGE_NOT_FOUND",
			"landing page not found or already deleted",
			http.StatusNotFound,
		)
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}

	switch pgErr.Code {
	case "23505":
		switch pgErr.ConstraintName {
		case "idx_landing_pages_organization_slug_active_unique":
			return coreerrors.New(
				"PAGE_SLUG_ALREADY_EXISTS",
				"landing page slug already exists in this organization",
				http.StatusConflict,
			)
		case "idx_landing_pages_organization_homepage_unique":
			return coreerrors.New(
				"PAGE_HOMEPAGE_ALREADY_EXISTS",
				"organization already has an active homepage landing page",
				http.StatusConflict,
			)
		default:
			return coreerrors.New(
				"PAGE_ALREADY_EXISTS",
				"landing page conflicts with an existing record",
				http.StatusConflict,
			)
		}
	case "23503":
		return coreerrors.New(
			"PAGE_REFERENCE_INVALID",
			"landing page references an invalid organization or user",
			http.StatusUnprocessableEntity,
		)
	case "23514":
		return coreerrors.New(
			"PAGE_DATA_INVALID",
			"landing page data violates a database constraint",
			http.StatusUnprocessableEntity,
		)
	default:
		return err
	}
}
