package service

import (
	"context"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
)

type SaveDocumentParams struct {
	PageID  string
	Project map[string]any
	HTML    string
	CSS     string
	ActorID string
}

// DocumentService owns the GrapesJS working copy of a landing page. It validates
// shape and size only — HTML/CSS are NOT sanitized here (they never reach a
// browser un-sanitized: the editor re-hydrates from Project, and Publish
// sanitizes into the version snapshot).
type DocumentService interface {
	Get(ctx context.Context, scope coretenant.Scope, pageID string) (domain.LandingPageDocument, error)
	Save(ctx context.Context, scope coretenant.Scope, params SaveDocumentParams) (domain.LandingPageDocument, error)
}
