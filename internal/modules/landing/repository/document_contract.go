package repository

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type UpsertDocumentParams struct {
	LandingPageID string
	Project       map[string]any
	HTML          string
	CSS           string
	UpdatedBy     string
}

// DocumentRepository is the tenant-owned data contract for landing_page_documents.
type DocumentRepository interface {
	GetByPageID(context.Context, coretenant.Scope, string) (domain.LandingPageDocument, error)
	Upsert(context.Context, coretenant.Scope, UpsertDocumentParams) (domain.LandingPageDocument, error)
}
