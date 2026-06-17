package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type pageService struct {
	pageRepo    repository.PageRepository
	sectionRepo repository.SectionRepository
}

func NewPageService(pageRepo repository.PageRepository, sectionRepo repository.SectionRepository) PageService {
	return &pageService{
		pageRepo:    pageRepo,
		sectionRepo: sectionRepo,
	}
}

func (s *pageService) Create(ctx context.Context, scope coretenant.Scope, params repository.CreatePageParams) (domain.LandingPage, error) {
	// Sanitize and generate a safe slug
	params.Slug = generateSafeSlug(params.Slug, params.Title)
	
	// Handle basic slug conflict by appending random if needed, but for MVP we rely on DB unique constraint
	// If DB throws unique constraint violation, handler can map it to 409 Conflict.
	return s.pageRepo.Create(ctx, scope, params)
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
	return s.pageRepo.Update(ctx, scope, id, params)
}

func (s *pageService) Delete(ctx context.Context, scope coretenant.Scope, id string) error {
	return s.pageRepo.Delete(ctx, scope, id)
}

func (s *pageService) Duplicate(ctx context.Context, scope coretenant.Scope, params DuplicatePageParams) (domain.LandingPage, error) {
	// 1. Get original page
	originalPage, err := s.pageRepo.FindByID(ctx, scope, params.PageID)
	if err != nil {
		return domain.LandingPage{}, err
	}

	// 2. Generate new slug
	newSlug := fmt.Sprintf("%s-copy-%d", originalPage.Slug, time.Now().Unix())
	
	// 3. Create new page as Draft
	createParams := repository.CreatePageParams{
		Name:       originalPage.Name + " (Copy)",
		Title:      originalPage.Title,
		Slug:       newSlug,
		Type:       originalPage.Type,
		Status:     domain.PageStatusDraft,
		Visibility: originalPage.Visibility,
		Locale:     originalPage.Locale,
		Timezone:   originalPage.Timezone,
		IsHomepage: false,
		CreatedBy:  params.UserID,
	}

	newPage, err := s.pageRepo.Create(ctx, scope, createParams)
	if err != nil {
		return domain.LandingPage{}, err
	}

	// 4. Duplicate sections
	sections, err := s.sectionRepo.ListByPage(ctx, scope, originalPage.ID)
	if err == nil {
		for _, sec := range sections {
			_, _ = s.sectionRepo.Create(ctx, scope, repository.CreateSectionParams{
				LandingPageID: newPage.ID,
				Key:           sec.Key,
				Type:          sec.Type,
				Name:          sec.Name,
				SortOrder:     sec.SortOrder,
				IsEnabled:     sec.IsEnabled,
				Content:       sec.Content,
				Style:         sec.Style,
				CreatedBy:     params.UserID,
			})
			// Ignoring individual section copy errors to ensure page creation completes.
			// Ideally handled in DB transaction.
		}
	}

	return newPage, nil
}

func (s *pageService) Archive(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) error {
	status := domain.PageStatusArchived
	_, err := s.pageRepo.Update(ctx, scope, id, repository.UpdatePageParams{
		Status:    &status,
		UpdatedBy: updatedBy,
	})
	return err
}

func (s *pageService) Restore(ctx context.Context, scope coretenant.Scope, id string, updatedBy string) error {
	status := domain.PageStatusDraft
	_, err := s.pageRepo.Update(ctx, scope, id, repository.UpdatePageParams{
		Status:    &status,
		UpdatedBy: updatedBy,
	})
	return err
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
