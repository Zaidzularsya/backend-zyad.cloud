package service

import (
	"context"
	"net/http"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

var (
	ErrPageTemplateNotFound      = coreerrors.New("TEMPLATE_NOT_FOUND", "template tidak ditemukan", http.StatusNotFound)
	ErrPageTemplateSlugTaken     = coreerrors.New("TEMPLATE_SLUG_TAKEN", "slug template sudah dipakai", http.StatusConflict)
	ErrPageTemplateSourceMissing = coreerrors.New("TEMPLATE_SOURCE_MISSING", "halaman sumber template tidak ada", http.StatusConflict)
	ErrPageTemplatePlatformOnly  = coreerrors.New("PLATFORM_ONLY", "hanya untuk organisasi platform", http.StatusForbidden)
	ErrPageTemplateEmpty         = coreerrors.New("TEMPLATE_EMPTY", "halaman tidak berisi konten untuk dijadikan template", http.StatusUnprocessableEntity)
)

// PageTemplateViewer menggambarkan pemanggil. Scope adalah scope org pemanggil
// (dipakai untuk membaca halaman sumber); IsPlatform menentukan visibilitas.
type PageTemplateViewer struct {
	Scope      coretenant.Scope
	IsPlatform bool
	ActorID    string
}

type CreatePageTemplateInput struct {
	PageID, Name, Slug, Category, Description string
	SortOrder                                 int
}

type UpdatePageTemplateMetaInput struct {
	Name, Description, Category *string
	SortOrder                   *int
	Status                      *domain.PageTemplateStatus
	PlatformOnly                *bool
}

type PageTemplateService interface {
	ListPublished(ctx context.Context, v PageTemplateViewer) ([]domain.PageTemplate, error)
	Get(ctx context.Context, v PageTemplateViewer, id string) (domain.PageTemplate, error)
	ListAll(ctx context.Context, v PageTemplateViewer) ([]domain.PageTemplate, error)
	CreateFromPage(ctx context.Context, v PageTemplateViewer, in CreatePageTemplateInput) (domain.PageTemplate, error)
	RefreshFromPage(ctx context.Context, v PageTemplateViewer, id string) (domain.PageTemplate, error)
	CopyToPage(ctx context.Context, v PageTemplateViewer, id string) (domain.LandingPage, error)
	UpdateMeta(ctx context.Context, v PageTemplateViewer, id string, in UpdatePageTemplateMetaInput) (domain.PageTemplate, error)
}
