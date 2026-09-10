package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

type documentRepository struct {
	db *database.Pool
}

func NewDocumentRepository(db *database.Pool) DocumentRepository {
	return &documentRepository{db: db}
}

func (r *documentRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID()); err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *documentRepository) GetByPageID(ctx context.Context, scope coretenant.Scope, pageID string) (domain.LandingPageDocument, error) {
	if !scope.IsValid() {
		return domain.LandingPageDocument{}, coretenant.ErrInvalidScope
	}

	const query = `
		SELECT landing_page_id, organization_id, project, html, css,
		       updated_by, created_at, updated_at
		FROM landing_page_documents
		WHERE landing_page_id = $1
	`

	var doc domain.LandingPageDocument
	var updatedBy *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, pageID).Scan(
			&doc.LandingPageID, &doc.OrganizationID, &doc.Project, &doc.HTML, &doc.CSS,
			&updatedBy, &doc.CreatedAt, &doc.UpdatedAt,
		)
	})
	if err != nil {
		return domain.LandingPageDocument{}, err
	}
	if updatedBy != nil {
		doc.UpdatedBy = *updatedBy
	}
	return doc, nil
}

func (r *documentRepository) Upsert(ctx context.Context, scope coretenant.Scope, params UpsertDocumentParams) (domain.LandingPageDocument, error) {
	if !scope.IsValid() {
		return domain.LandingPageDocument{}, coretenant.ErrInvalidScope
	}

	project := params.Project
	if project == nil {
		project = map[string]any{}
	}

	var updatedBy interface{} = nil
	if params.UpdatedBy != "" {
		updatedBy = params.UpdatedBy
	}

	// organization_id is derived from the parent page so a caller cannot smuggle
	// a document under another tenant; the composite FK + RLS WITH CHECK also
	// reject a mismatch.
	const query = `
		INSERT INTO landing_page_documents (
			landing_page_id, organization_id, project, html, css, updated_by
		)
		SELECT $1, p.organization_id, $2::jsonb, $3, $4, $5
		FROM landing_pages p
		WHERE p.id = $1 AND p.organization_id = $6 AND p.deleted_at IS NULL
		ON CONFLICT (landing_page_id) DO UPDATE SET
			project = EXCLUDED.project,
			html = EXCLUDED.html,
			css = EXCLUDED.css,
			updated_by = EXCLUDED.updated_by,
			updated_at = NOW()
		RETURNING landing_page_id, organization_id, project, html, css,
		          updated_by, created_at, updated_at
	`

	var doc domain.LandingPageDocument
	var retUpdatedBy *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			params.LandingPageID,
			project,
			params.HTML,
			params.CSS,
			updatedBy,
			scope.OrganizationID(),
		).Scan(
			&doc.LandingPageID, &doc.OrganizationID, &doc.Project, &doc.HTML, &doc.CSS,
			&retUpdatedBy, &doc.CreatedAt, &doc.UpdatedAt,
		)
	})
	if err != nil {
		return domain.LandingPageDocument{}, err
	}
	if retUpdatedBy != nil {
		doc.UpdatedBy = *retUpdatedBy
	}
	return doc, nil
}
