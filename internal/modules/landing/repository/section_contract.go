package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type CreateSectionParams struct {
	LandingPageID string
	Key           string
	Type          domain.SectionType
	Name          string
	SortOrder     int
	IsEnabled     bool
	Content       map[string]any
	Style         map[string]any
	CreatedBy     string
}

type UpdateSectionParams struct {
	Name      *string
	IsEnabled *bool
	Content   map[string]any
	Style     map[string]any
	UpdatedBy string
}

type SectionReorderParam struct {
	ID        string
	SortOrder int
}

// ReplaceSectionItem is one entry of a full-page section replace. An empty ID
// means "create"; a non-empty ID (or a matching Key) means "update the existing
// row". Existing rows absent from the item list are soft-deleted. The final
// sort_order follows the slice order.
type ReplaceSectionItem struct {
	ID        string
	Key       string
	Type      domain.SectionType
	Name      string
	IsEnabled bool
	Content   map[string]any
	Style     map[string]any
}

// ReplaceAllParams is the payload for SectionRepository.ReplaceAll.
type ReplaceAllParams struct {
	LandingPageID string
	ActorID       string
	Items         []ReplaceSectionItem
}

type SectionRepository interface {
	Create(context.Context, coretenant.Scope, CreateSectionParams) (domain.LandingSection, error)
	FindByID(context.Context, coretenant.Scope, string) (domain.LandingSection, error)
	ListByPage(context.Context, coretenant.Scope, string) ([]domain.LandingSection, error)
	Update(context.Context, coretenant.Scope, string, UpdateSectionParams) (domain.LandingSection, error)
	Reorder(context.Context, coretenant.Scope, string, []SectionReorderParam) error
	// ReplaceAll upserts the given sections and soft-deletes any existing
	// section of the page not present in Items, atomically in one transaction.
	ReplaceAll(context.Context, coretenant.Scope, ReplaceAllParams) ([]domain.LandingSection, error)
	Delete(context.Context, coretenant.Scope, string) error
}
