package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

type SectionService interface {
	Create(context.Context, coretenant.Scope, coretenant.OrganizationType, repository.CreateSectionParams) (domain.LandingSection, error)
	Get(context.Context, coretenant.Scope, string) (domain.LandingSection, error)
	ListByPage(context.Context, coretenant.Scope, string) ([]domain.LandingSection, error)
	Update(context.Context, coretenant.Scope, coretenant.OrganizationType, string, repository.UpdateSectionParams) (domain.LandingSection, error)
	Delete(context.Context, coretenant.Scope, string) error
	Toggle(context.Context, coretenant.Scope, string, bool, string) error
	Reorder(context.Context, coretenant.Scope, string, []repository.SectionReorderParam) error
}
