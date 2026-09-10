package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	coreerrors "zyad.cloud/internal/core/errors"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/modules/landing/repository"
)

const (
	// maxDocumentProjectBytes caps the serialized GrapesJS project JSON.
	maxDocumentProjectBytes = 4 * 1024 * 1024
	// maxDocumentMarkupBytes caps the exported HTML and (separately) the CSS.
	maxDocumentMarkupBytes = 2 * 1024 * 1024
)

var errDocumentPageNotFound = coreerrors.New(
	"PAGE_NOT_FOUND",
	"landing page not found or already deleted",
	http.StatusNotFound,
)

type documentService struct {
	documentRepo repository.DocumentRepository
	pageRepo     repository.PageRepository
}

func NewDocumentService(documentRepo repository.DocumentRepository, pageRepo repository.PageRepository) DocumentService {
	return &documentService{documentRepo: documentRepo, pageRepo: pageRepo}
}

func (s *documentService) Get(ctx context.Context, scope coretenant.Scope, pageID string) (domain.LandingPageDocument, error) {
	if !scope.IsValid() {
		return domain.LandingPageDocument{}, coretenant.ErrInvalidScope
	}

	if _, err := s.pageRepo.FindByID(ctx, scope, pageID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.LandingPageDocument{}, errDocumentPageNotFound
		}
		return domain.LandingPageDocument{}, err
	}

	doc, err := s.documentRepo.GetByPageID(ctx, scope, pageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Page exists but has never been saved — hand back an empty doc so
			// the editor loads blank.
			return domain.LandingPageDocument{
				LandingPageID:  pageID,
				OrganizationID: scope.OrganizationID(),
				Project:        map[string]any{},
			}, nil
		}
		return domain.LandingPageDocument{}, err
	}
	return doc, nil
}

func (s *documentService) Save(ctx context.Context, scope coretenant.Scope, params SaveDocumentParams) (domain.LandingPageDocument, error) {
	if !scope.IsValid() {
		return domain.LandingPageDocument{}, coretenant.ErrInvalidScope
	}

	project := params.Project
	if project == nil {
		project = map[string]any{}
	}

	raw, err := json.Marshal(project)
	if err != nil {
		return domain.LandingPageDocument{}, coreerrors.New(
			"VALIDATION_ERROR", "project harus berupa JSON object yang valid", http.StatusUnprocessableEntity,
		)
	}
	if len(raw) > maxDocumentProjectBytes {
		return domain.LandingPageDocument{}, coreerrors.New(
			"VALIDATION_ERROR", "project melebihi batas ukuran", http.StatusRequestEntityTooLarge,
		)
	}
	if len(params.HTML) > maxDocumentMarkupBytes || len(params.CSS) > maxDocumentMarkupBytes {
		return domain.LandingPageDocument{}, coreerrors.New(
			"VALIDATION_ERROR", "html/css melebihi batas ukuran", http.StatusRequestEntityTooLarge,
		)
	}

	doc, err := s.documentRepo.Upsert(ctx, scope, repository.UpsertDocumentParams{
		LandingPageID: params.PageID,
		Project:       project,
		HTML:          params.HTML,
		CSS:           params.CSS,
		UpdatedBy:     params.ActorID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.LandingPageDocument{}, errDocumentPageNotFound
		}
		return domain.LandingPageDocument{}, err
	}
	return doc, nil
}
