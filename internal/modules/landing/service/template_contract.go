package service

import (
	"context"

	"errors"

	"zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

var (
	ErrInvalidSectionType = errors.New("invalid section type")
)

// TemplateService interface defines operations for Reusable Section Templates
type TemplateService interface {
	Create(ctx context.Context, scope tenant.Scope, params repository.CreateSectionTemplateParams) (domain.SectionTemplate, error)
	FindByID(ctx context.Context, scope tenant.Scope, id string) (domain.SectionTemplate, error)
	List(ctx context.Context, scope tenant.Scope, sectionType domain.SectionType) ([]domain.SectionTemplate, error)
	Update(ctx context.Context, scope tenant.Scope, id string, params repository.UpdateSectionTemplateParams) (domain.SectionTemplate, error)
	Delete(ctx context.Context, scope tenant.Scope, id string, updatedBy string) error
	
	// InstantiateToPage copies the template into a page section
	InstantiateToPage(ctx context.Context, scope tenant.Scope, templateID string, pageID string, sectionKey string, sortOrder int, createdBy string) (domain.LandingSection, error)
}
