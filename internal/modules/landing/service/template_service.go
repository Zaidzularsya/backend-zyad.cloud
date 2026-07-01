package service

import (
	"context"
	"errors"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

var (
	ErrTemplateNotFound = errors.New("template not found")
)

type templateService struct {
	reusableRepo repository.ReusableRepository
	sectionRepo  repository.SectionRepository
	quotaGuard   LandingSectionQuotaGuard
}

type TemplateServiceOption func(*templateService)

func WithTemplateSectionQuotaGuard(guard LandingSectionQuotaGuard) TemplateServiceOption {
	return func(service *templateService) {
		service.quotaGuard = guard
	}
}

func NewTemplateService(
	reusableRepo repository.ReusableRepository,
	sectionRepo repository.SectionRepository,
	options ...TemplateServiceOption,
) TemplateService {
	service := &templateService{
		reusableRepo: reusableRepo,
		sectionRepo:  sectionRepo,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *templateService) Create(ctx context.Context, scope tenant.Scope, params repository.CreateSectionTemplateParams) (domain.SectionTemplate, error) {
	if !scope.IsValid() {
		return domain.SectionTemplate{}, tenant.ErrInvalidScope
	}

	if !params.SectionType.IsValid() {
		return domain.SectionTemplate{}, ErrInvalidSectionType
	}

	return s.reusableRepo.CreateSectionTemplate(ctx, scope, params)
}

func (s *templateService) FindByID(ctx context.Context, scope tenant.Scope, id string) (domain.SectionTemplate, error) {
	if !scope.IsValid() {
		return domain.SectionTemplate{}, tenant.ErrInvalidScope
	}

	return s.reusableRepo.GetSectionTemplate(ctx, scope, id)
}

func (s *templateService) List(ctx context.Context, scope tenant.Scope, sectionType domain.SectionType) ([]domain.SectionTemplate, error) {
	if !scope.IsValid() {
		return nil, tenant.ErrInvalidScope
	}

	if sectionType != "" && !sectionType.IsValid() {
		return nil, ErrInvalidSectionType
	}

	return s.reusableRepo.ListSectionTemplates(ctx, scope, sectionType)
}

func (s *templateService) Update(ctx context.Context, scope tenant.Scope, id string, params repository.UpdateSectionTemplateParams) (domain.SectionTemplate, error) {
	if !scope.IsValid() {
		return domain.SectionTemplate{}, tenant.ErrInvalidScope
	}

	return s.reusableRepo.UpdateSectionTemplate(ctx, scope, id, params)
}

func (s *templateService) Delete(ctx context.Context, scope tenant.Scope, id string, updatedBy string) error {
	if !scope.IsValid() {
		return tenant.ErrInvalidScope
	}

	return s.reusableRepo.DeleteSectionTemplate(ctx, scope, id, updatedBy)
}

func (s *templateService) InstantiateToPage(ctx context.Context, scope tenant.Scope, templateID string, pageID string, sectionKey string, sortOrder int, createdBy string) (domain.LandingSection, error) {
	if !scope.IsValid() {
		return domain.LandingSection{}, tenant.ErrInvalidScope
	}
	if err := s.requireInstantiateQuota(ctx, scope, pageID); err != nil {
		return domain.LandingSection{}, err
	}

	// 1. Get the template
	template, err := s.reusableRepo.GetSectionTemplate(ctx, scope, templateID)
	if err != nil {
		return domain.LandingSection{}, err // let repo error propagate, usually pgx.ErrNoRows
	}

	// 2. Create Section using template's config
	sectionParams := repository.CreateSectionParams{
		LandingPageID: pageID,
		Key:           sectionKey,
		Type:          template.SectionType,
		Name:          template.Name,
		SortOrder:     sortOrder,
		IsEnabled:     true,
		Content:       template.Content,
		Style:         template.Style,
		CreatedBy:     createdBy,
	}

	return s.sectionRepo.Create(ctx, scope, sectionParams)
}

func (s *templateService) requireInstantiateQuota(
	ctx context.Context,
	scope tenant.Scope,
	pageID string,
) error {
	if s.quotaGuard == nil {
		return nil
	}
	sections, err := s.sectionRepo.ListByPage(ctx, scope, pageID)
	if err != nil {
		return err
	}
	return s.quotaGuard.RequireQuotaValue(
		ctx,
		scope.OrganizationID(),
		domain.FeatureLandingMaxSections,
		"limit",
		int64(len(sections)),
		1,
	)
}
