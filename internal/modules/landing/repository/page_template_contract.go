package repository

import (
	"context"
	"errors"

	"zyad.cloud/internal/modules/landing/domain"
)

// ErrPageTemplateSlugTaken dipetakan dari unique violation (23505) pada slug.
var ErrPageTemplateSlugTaken = errors.New("page template slug taken")

type PageTemplateListFilter struct {
	Statuses            []domain.PageTemplateStatus // kosong = semua
	IncludePlatformOnly bool
}

type CreatePageTemplateParams struct {
	Slug, Name, Description, Category string
	SortOrder                         int
	Status                            domain.PageTemplateStatus
	PlatformOnly                      bool
	HTML, CSS                         string
	Source                            domain.PageTemplateSource
	SeedVersion                       *int
	SourcePageID                      *string
	CreatedBy                         string // "" → NULL
}

// UpdatePageTemplateParams: nil berarti kolom tidak diubah.
type UpdatePageTemplateParams struct {
	Name, Description, Category *string
	SortOrder                   *int
	Status                      *domain.PageTemplateStatus
	PlatformOnly                *bool
	HTML, CSS                   *string
	SeedVersion                 *int
	SeedModified                *bool
	SourcePageID                *string
	UpdatedBy                   string
}

// PlatformPageTemplateRepository mengakses tabel global landing_page_templates
// (tanpa scope tenant dan tanpa RLS; visibilitas dijaga di service).
type PlatformPageTemplateRepository interface {
	// List tidak mengisi HTML/CSS; urut sort_order, name.
	List(ctx context.Context, f PageTemplateListFilter) ([]domain.PageTemplate, error)
	// FindByID mengembalikan pgx.ErrNoRows bila tidak ada (termasuk id bukan uuid).
	FindByID(ctx context.Context, id string) (domain.PageTemplate, error)
	FindBySlug(ctx context.Context, slug string) (domain.PageTemplate, error)
	Create(ctx context.Context, p CreatePageTemplateParams) (domain.PageTemplate, error)
	Update(ctx context.Context, id string, p UpdatePageTemplateParams) (domain.PageTemplate, error)
	WithSeedLock(ctx context.Context, fn func(ctx context.Context) error) error
}
