package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type pageTemplateService struct {
	repo  repository.PlatformPageTemplateRepository
	docs  repository.DocumentRepository
	pages repository.PageRepository
}

func NewPageTemplateService(
	repo repository.PlatformPageTemplateRepository,
	docs repository.DocumentRepository,
	pages repository.PageRepository,
) PageTemplateService {
	return &pageTemplateService{repo: repo, docs: docs, pages: pages}
}

func requirePlatform(v PageTemplateViewer) error {
	if !v.IsPlatform {
		return ErrPageTemplatePlatformOnly
	}
	return nil
}

func (s *pageTemplateService) ListPublished(ctx context.Context, v PageTemplateViewer) ([]domain.PageTemplate, error) {
	return s.repo.List(ctx, repository.PageTemplateListFilter{
		Statuses:            []domain.PageTemplateStatus{domain.PageTemplateStatusPublished},
		IncludePlatformOnly: v.IsPlatform,
	})
}

func (s *pageTemplateService) ListAll(ctx context.Context, v PageTemplateViewer) ([]domain.PageTemplate, error) {
	if err := requirePlatform(v); err != nil {
		return nil, err
	}
	return s.repo.List(ctx, repository.PageTemplateListFilter{IncludePlatformOnly: true})
}

func (s *pageTemplateService) find(ctx context.Context, id string) (domain.PageTemplate, error) {
	t, err := s.repo.FindByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PageTemplate{}, ErrPageTemplateNotFound
	}
	return t, err
}

// Get menyembunyikan template yang tidak boleh dilihat org customer sebagai
// 404 (bukan 403) supaya keberadaannya tidak bocor.
func (s *pageTemplateService) Get(ctx context.Context, v PageTemplateViewer, id string) (domain.PageTemplate, error) {
	t, err := s.find(ctx, id)
	if err != nil {
		return domain.PageTemplate{}, err
	}
	if !v.IsPlatform && (t.Status != domain.PageTemplateStatusPublished || t.PlatformOnly) {
		return domain.PageTemplate{}, ErrPageTemplateNotFound
	}
	return t, nil
}

// sanitizedPageMarkup membaca dokumen halaman GrapesJS milik scope pemanggil,
// menyanitasinya, dan menolak hasil yang kosong.
func (s *pageTemplateService) sanitizedPageMarkup(ctx context.Context, v PageTemplateViewer, pageID string) (string, string, error) {
	page, err := s.pages.FindByID(ctx, v.Scope, pageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrPageTemplateNotFound
		}
		return "", "", err
	}
	if page.Builder != domain.PageBuilderGrapesJS {
		return "", "", ErrPageTemplateNotFound
	}
	doc, err := s.docs.GetByPageID(ctx, v.Scope, pageID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", err
	}
	if len(doc.HTML) > maxDocumentMarkupBytes || len(doc.CSS) > maxDocumentMarkupBytes {
		return "", "", templateValidationError("isi halaman terlalu besar untuk dijadikan template")
	}
	html, css := SanitizeGrapesHTML(doc.HTML), SanitizeGrapesCSS(doc.CSS)
	if IsEmptyTemplateMarkup(html) {
		return "", "", ErrPageTemplateEmpty
	}
	return html, css, nil
}

func (s *pageTemplateService) CreateFromPage(ctx context.Context, v PageTemplateViewer, in CreatePageTemplateInput) (domain.PageTemplate, error) {
	if err := requirePlatform(v); err != nil {
		return domain.PageTemplate{}, err
	}
	if err := ValidatePageTemplateMeta(in.Name, in.Slug, in.Category, in.Description, in.SortOrder); err != nil {
		return domain.PageTemplate{}, err
	}
	html, css, err := s.sanitizedPageMarkup(ctx, v, in.PageID)
	if err != nil {
		return domain.PageTemplate{}, err
	}
	pageID := in.PageID
	t, err := s.repo.Create(ctx, repository.CreatePageTemplateParams{
		Slug: in.Slug, Name: in.Name, Description: in.Description, Category: in.Category,
		SortOrder: in.SortOrder, Status: domain.PageTemplateStatusDraft,
		PlatformOnly: LockPlatformOnly(html, false),
		HTML:         html, CSS: css,
		Source: domain.PageTemplateSourceAdmin, SourcePageID: &pageID, CreatedBy: v.ActorID,
	})
	if errors.Is(err, repository.ErrPageTemplateSlugTaken) {
		return domain.PageTemplate{}, ErrPageTemplateSlugTaken
	}
	return t, err
}

func (s *pageTemplateService) RefreshFromPage(ctx context.Context, v PageTemplateViewer, id string) (domain.PageTemplate, error) {
	if err := requirePlatform(v); err != nil {
		return domain.PageTemplate{}, err
	}
	current, err := s.find(ctx, id)
	if err != nil {
		return domain.PageTemplate{}, err
	}
	if current.SourcePageID == nil {
		return domain.PageTemplate{}, ErrPageTemplateSourceMissing
	}
	html, css, err := s.sanitizedPageMarkup(ctx, v, *current.SourcePageID)
	if errors.Is(err, ErrPageTemplateNotFound) {
		return domain.PageTemplate{}, ErrPageTemplateSourceMissing
	}
	if err != nil {
		return domain.PageTemplate{}, err
	}
	params := repository.UpdatePageTemplateParams{
		HTML: &html, CSS: &css, UpdatedBy: v.ActorID,
	}
	platformOnly := LockPlatformOnly(html, current.PlatformOnly)
	params.PlatformOnly = &platformOnly
	if current.Source == domain.PageTemplateSourceSeed {
		modified := true
		params.SeedModified = &modified
	}
	return s.repo.Update(ctx, id, params)
}

func (s *pageTemplateService) UpdateMeta(ctx context.Context, v PageTemplateViewer, id string, in UpdatePageTemplateMetaInput) (domain.PageTemplate, error) {
	if err := requirePlatform(v); err != nil {
		return domain.PageTemplate{}, err
	}
	current, err := s.find(ctx, id)
	if err != nil {
		return domain.PageTemplate{}, err
	}
	name, description, category, sortOrder := current.Name, current.Description, current.Category, current.SortOrder
	if in.Name != nil {
		name = *in.Name
	}
	if in.Description != nil {
		description = *in.Description
	}
	if in.Category != nil {
		category = *in.Category
	}
	if in.SortOrder != nil {
		sortOrder = *in.SortOrder
	}
	if err := ValidatePageTemplateMeta(name, current.Slug, category, description, sortOrder); err != nil {
		return domain.PageTemplate{}, err
	}
	if in.Status != nil && !in.Status.IsValid() {
		return domain.PageTemplate{}, templateValidationError("status tidak valid")
	}
	requested := current.PlatformOnly
	if in.PlatformOnly != nil {
		requested = *in.PlatformOnly
	}
	platformOnly := LockPlatformOnly(current.HTML, requested)
	return s.repo.Update(ctx, id, repository.UpdatePageTemplateParams{
		Name: in.Name, Description: in.Description, Category: in.Category, SortOrder: in.SortOrder,
		Status: in.Status, PlatformOnly: &platformOnly, UpdatedBy: v.ActorID,
	})
}

func (s *pageTemplateService) CopyToPage(ctx context.Context, v PageTemplateViewer, id string) (domain.LandingPage, error) {
	if err := requirePlatform(v); err != nil {
		return domain.LandingPage{}, err
	}
	tpl, err := s.find(ctx, id)
	if err != nil {
		return domain.LandingPage{}, err
	}
	var lookupErr error
	slug := NextWorkPageSlug(tpl.Slug, func(candidate string) bool {
		_, err := s.pages.FindBySlug(ctx, v.Scope, candidate)
		if errors.Is(err, pgx.ErrNoRows) {
			return false
		}
		if err != nil && lookupErr == nil {
			lookupErr = err
		}
		return true
	})
	if lookupErr != nil {
		return domain.LandingPage{}, lookupErr
	}
	name := fmt.Sprintf("Template – %s", tpl.Name)
	page, err := s.pages.Create(ctx, v.Scope, repository.CreatePageParams{
		Name: name, Title: name, Slug: slug, Type: domain.PageTypeCampaign,
		Builder: domain.PageBuilderGrapesJS, Status: domain.PageStatusDraft,
		Visibility: domain.PageVisibilityPrivate, Locale: "id-ID", Timezone: "Asia/Jakarta",
		IsTemplate: false, CreatedBy: v.ActorID,
	})
	if err != nil {
		return domain.LandingPage{}, err
	}
	if _, err := s.docs.Upsert(ctx, v.Scope, repository.UpsertDocumentParams{
		LandingPageID: page.ID, Project: map[string]any{}, HTML: tpl.HTML, CSS: tpl.CSS, UpdatedBy: v.ActorID,
	}); err != nil {
		return domain.LandingPage{}, err
	}
	if _, err := s.repo.Update(ctx, id, repository.UpdatePageTemplateParams{SourcePageID: &page.ID, UpdatedBy: v.ActorID}); err != nil {
		return domain.LandingPage{}, err
	}
	return page, nil
}
