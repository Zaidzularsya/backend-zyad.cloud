package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

type activationRepository struct {
	db *database.Pool
}

func NewActivationRepository(db *database.Pool) ActivationRepository {
	return &activationRepository{db: db}
}

const activationColumns = `
	id, organization_id, page_id, invoice_id, buyer_name, buyer_contact, buyer_email,
	price::text, duration_value, duration_unit, preview_token_hash, invoice_due_at,
	status, paid_at, created_by, created_at`

func scanActivation(row pgx.Row) (domain.LandingPageActivation, error) {
	var a domain.LandingPageActivation
	err := row.Scan(
		&a.ID, &a.OrganizationID, &a.PageID, &a.InvoiceID, &a.BuyerName, &a.BuyerContact, &a.BuyerEmail,
		&a.Price, &a.DurationValue, &a.DurationUnit, &a.PreviewTokenHash, &a.InvoiceDueAt,
		&a.Status, &a.PaidAt, &a.CreatedBy, &a.CreatedAt,
	)
	return a, err
}

func (r *activationRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID()); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *activationRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateActivationParams) (domain.LandingPageActivation, error) {
	var createdBy any
	if params.CreatedBy != "" {
		createdBy = params.CreatedBy
	}
	// The page must belong to the scope; the composite FK enforces it too.
	query := `
		INSERT INTO landing_page_activations (
			organization_id, page_id, invoice_id, buyer_name, buyer_contact, buyer_email,
			price, duration_value, duration_unit, preview_token_hash, invoice_due_at,
			status, created_by
		)
		SELECT $1::uuid, p.id, $3::uuid, $4::varchar, $5::varchar, $6::varchar, $7::numeric, $8::integer,
			$9::varchar, $10::text, $11::timestamp, 'open', $12::uuid
		FROM landing_pages p
		WHERE p.id = $2::uuid AND p.organization_id = $1::uuid AND p.deleted_at IS NULL
		RETURNING ` + activationColumns

	var a domain.LandingPageActivation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		a, err = scanActivation(tx.QueryRow(ctx, query,
			scope.OrganizationID(), params.PageID, params.InvoiceID, params.BuyerName, params.BuyerContact,
			params.BuyerEmail, params.Price, params.DurationValue, params.DurationUnit,
			params.PreviewTokenHash, params.InvoiceDueAt.UTC(), createdBy,
		))
		return err
	})
	return a, err
}

func (r *activationRepository) FindOpenByPage(ctx context.Context, scope coretenant.Scope, pageID string) (domain.LandingPageActivation, error) {
	query := `SELECT ` + activationColumns + `
		FROM landing_page_activations
		WHERE organization_id = $1 AND page_id = $2 AND status = 'open'`

	var a domain.LandingPageActivation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		a, err = scanActivation(tx.QueryRow(ctx, query, scope.OrganizationID(), pageID))
		return err
	})
	return a, err
}

func (r *activationRepository) ListOpenByPages(ctx context.Context, scope coretenant.Scope, pageIDs []string) (map[string]domain.LandingPageActivation, error) {
	result := make(map[string]domain.LandingPageActivation, len(pageIDs))
	if len(pageIDs) == 0 {
		return result, nil
	}
	query := `SELECT ` + activationColumns + `
		FROM landing_page_activations
		WHERE organization_id = $1 AND page_id = ANY($2::uuid[]) AND status = 'open'`

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID(), pageIDs)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanActivation(rows)
			if err != nil {
				return err
			}
			result[a.PageID] = a
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *activationRepository) FindByTokenHash(ctx context.Context, scope coretenant.Scope, tokenHash string) (domain.LandingPageActivation, error) {
	query := `SELECT ` + activationColumns + `
		FROM landing_page_activations
		WHERE organization_id = $1 AND preview_token_hash = $2`

	var a domain.LandingPageActivation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		a, err = scanActivation(tx.QueryRow(ctx, query, scope.OrganizationID(), tokenHash))
		return err
	})
	return a, err
}

func (r *activationRepository) MarkPaid(ctx context.Context, scope coretenant.Scope, id string, paidAt time.Time) (domain.LandingPageActivation, error) {
	query := `
		UPDATE landing_page_activations
		SET status = 'paid', paid_at = $3
		WHERE organization_id = $1 AND id = $2 AND status = 'open'
		RETURNING ` + activationColumns

	var a domain.LandingPageActivation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		a, err = scanActivation(tx.QueryRow(ctx, query, scope.OrganizationID(), id, paidAt.UTC()))
		return err
	})
	return a, err
}

func (r *activationRepository) Void(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingPageActivation, error) {
	query := `
		UPDATE landing_page_activations
		SET status = 'void'
		WHERE organization_id = $1 AND id = $2 AND status = 'open'
		RETURNING ` + activationColumns

	var a domain.LandingPageActivation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		a, err = scanActivation(tx.QueryRow(ctx, query, scope.OrganizationID(), id))
		return err
	})
	return a, err
}
