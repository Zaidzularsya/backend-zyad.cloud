package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

// ── stubs ──────────────────────────────────────────────────────────────────

type documentRepoStub struct {
	getResult    domain.LandingPageDocument
	getErr       error
	upsertResult domain.LandingPageDocument
	upsertErr    error
	upsertParams repository.UpsertDocumentParams
	upsertCalled bool
}

func (s *documentRepoStub) GetByPageID(context.Context, coretenant.Scope, string) (domain.LandingPageDocument, error) {
	return s.getResult, s.getErr
}

func (s *documentRepoStub) Upsert(_ context.Context, _ coretenant.Scope, p repository.UpsertDocumentParams) (domain.LandingPageDocument, error) {
	s.upsertCalled = true
	s.upsertParams = p
	return s.upsertResult, s.upsertErr
}

type documentPageRepoStub struct {
	findErr error
}

func (s *documentPageRepoStub) Create(context.Context, coretenant.Scope, repository.CreatePageParams) (domain.LandingPage, error) {
	return domain.LandingPage{}, nil
}
func (s *documentPageRepoStub) FindByID(context.Context, coretenant.Scope, string) (domain.LandingPage, error) {
	return domain.LandingPage{ID: "page-1", Builder: domain.PageBuilderGrapesJS}, s.findErr
}
func (s *documentPageRepoStub) FindBySlug(context.Context, coretenant.Scope, string) (domain.LandingPage, error) {
	return domain.LandingPage{}, nil
}
func (s *documentPageRepoStub) List(context.Context, coretenant.Scope, repository.PageListFilter) ([]domain.LandingPage, int64, error) {
	return nil, 0, nil
}
func (s *documentPageRepoStub) Update(context.Context, coretenant.Scope, string, repository.UpdatePageParams) (domain.LandingPage, error) {
	return domain.LandingPage{}, nil
}
func (s *documentPageRepoStub) Delete(context.Context, coretenant.Scope, string) error { return nil }

// ── tests ──────────────────────────────────────────────────────────────────

func TestDocumentServiceGetReturnsEmptyDocWhenNeverSaved(t *testing.T) {
	svc := NewDocumentService(&documentRepoStub{getErr: pgx.ErrNoRows}, &documentPageRepoStub{})

	doc, err := svc.Get(context.Background(), mustLandingScope(t), "page-1")
	if err != nil {
		t.Fatalf("Get: unexpected error %v", err)
	}
	if doc.LandingPageID != "page-1" || doc.Project == nil || doc.HTML != "" {
		t.Fatalf("expected empty doc for the page, got %#v", doc)
	}
}

func TestDocumentServiceGet404WhenPageMissing(t *testing.T) {
	svc := NewDocumentService(&documentRepoStub{}, &documentPageRepoStub{findErr: pgx.ErrNoRows})

	_, err := svc.Get(context.Background(), mustLandingScope(t), "nope")
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "PAGE_NOT_FOUND" {
		t.Fatalf("expected PAGE_NOT_FOUND, got %v", err)
	}
}

func TestDocumentServiceSavePassesParamsThrough(t *testing.T) {
	repo := &documentRepoStub{upsertResult: domain.LandingPageDocument{LandingPageID: "page-1", HTML: "<p>ok</p>"}}
	svc := NewDocumentService(repo, &documentPageRepoStub{})

	_, err := svc.Save(context.Background(), mustLandingScope(t), SaveDocumentParams{
		PageID:  "page-1",
		Project: map[string]any{"pages": []any{}},
		HTML:    "<p>ok</p>",
		CSS:     "p{color:red}",
		ActorID: "user-1",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !repo.upsertCalled || repo.upsertParams.LandingPageID != "page-1" || repo.upsertParams.UpdatedBy != "user-1" {
		t.Fatalf("upsert not called with expected params: %#v", repo.upsertParams)
	}
}

func TestDocumentServiceSaveRejectsOversizedMarkup(t *testing.T) {
	svc := NewDocumentService(&documentRepoStub{}, &documentPageRepoStub{})

	_, err := svc.Save(context.Background(), mustLandingScope(t), SaveDocumentParams{
		PageID: "page-1",
		HTML:   strings.Repeat("a", maxDocumentMarkupBytes+1),
	})
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR for oversized html, got %v", err)
	}
}

func TestDocumentServiceSaveMapsMissingPageToNotFound(t *testing.T) {
	svc := NewDocumentService(&documentRepoStub{upsertErr: pgx.ErrNoRows}, &documentPageRepoStub{})

	_, err := svc.Save(context.Background(), mustLandingScope(t), SaveDocumentParams{PageID: "ghost"})
	var appErr *coreerrors.AppError
	if !errors.As(err, &appErr) || appErr.Code != "PAGE_NOT_FOUND" {
		t.Fatalf("expected PAGE_NOT_FOUND, got %v", err)
	}
}
