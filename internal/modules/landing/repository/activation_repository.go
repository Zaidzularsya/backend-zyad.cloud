package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

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

// insertActivationQuery inserts only when the page belongs to the scope; the
// composite FK enforces it too.
const insertActivationQuery = `
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

// oneOpenPerPageIndex is the partial unique index of migration 000163.
const oneOpenPerPageIndex = "idx_landing_page_activations_one_open_per_page"

func insertActivation(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, params CreateActivationParams) (domain.LandingPageActivation, error) {
	var createdBy any
	if params.CreatedBy != "" {
		createdBy = params.CreatedBy
	}
	a, err := scanActivation(tx.QueryRow(ctx, insertActivationQuery,
		scope.OrganizationID(), params.PageID, params.InvoiceID, params.BuyerName, params.BuyerContact,
		params.BuyerEmail, params.Price, params.DurationValue, params.DurationUnit,
		params.PreviewTokenHash, params.InvoiceDueAt.UTC(), createdBy,
	))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == oneOpenPerPageIndex {
		return domain.LandingPageActivation{}, ErrActivationOpen
	}
	return a, err
}

func (r *activationRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateActivationParams) (domain.LandingPageActivation, error) {
	var a domain.LandingPageActivation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		a, err = insertActivation(ctx, tx, scope, params)
		return err
	})
	return a, err
}

func (r *activationRepository) CreateReplacingOverdue(ctx context.Context, scope coretenant.Scope, params CreateActivationParams, now time.Time) (domain.LandingPageActivation, *domain.LandingPageActivation, error) {
	lockOpen := `SELECT ` + activationColumns + `
		FROM landing_page_activations
		WHERE organization_id = $1 AND page_id = $2 AND status = 'open'
		FOR UPDATE`
	voidOne := `
		UPDATE landing_page_activations
		SET status = 'void'
		WHERE organization_id = $1 AND id = $2 AND status = 'open'
		RETURNING ` + activationColumns

	var created domain.LandingPageActivation
	var voided *domain.LandingPageActivation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		open, err := scanActivation(tx.QueryRow(ctx, lockOpen, scope.OrganizationID(), params.PageID))
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return err
		case open.InvoiceDueAt.After(now.UTC()):
			return ErrActivationOpen
		default:
			v, err := scanActivation(tx.QueryRow(ctx, voidOne, scope.OrganizationID(), open.ID))
			if err != nil {
				return err
			}
			voided = &v
		}
		created, err = insertActivation(ctx, tx, scope, params)
		return err
	})
	if err != nil {
		return domain.LandingPageActivation{}, nil, err
	}
	return created, voided, nil
}

func (r *activationRepository) SetInvoice(ctx context.Context, scope coretenant.Scope, id, invoiceID string) (domain.LandingPageActivation, error) {
	query := `
		UPDATE landing_page_activations
		SET invoice_id = $3
		WHERE organization_id = $1 AND id = $2 AND status = 'open' AND invoice_id IS NULL
		RETURNING ` + activationColumns

	var a domain.LandingPageActivation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		a, err = scanActivation(tx.QueryRow(ctx, query, scope.OrganizationID(), id, invoiceID))
		return err
	})
	return a, err
}

func (r *activationRepository) ExistsForInvoice(ctx context.Context, scope coretenant.Scope, invoiceID string) (bool, error) {
	query := `SELECT EXISTS (
		SELECT 1 FROM landing_page_activations
		WHERE organization_id = $1 AND invoice_id = $2
	)`
	var exists bool
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, scope.OrganizationID(), invoiceID).Scan(&exists)
	})
	return exists, err
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

func (r *activationRepository) SettleByInvoice(ctx context.Context, scope coretenant.Scope, invoiceID string, paidAt time.Time) (ActivationSettlement, error) {
	lockActivation := `SELECT ` + activationColumns + `
		FROM landing_page_activations
		WHERE organization_id = $1 AND invoice_id = $2
		FOR UPDATE`
	// Lock order is always activation then page; the page lock serializes two
	// activations of the same page so active_until is computed from fresh state.
	lockPage := `SELECT id, organization_id, status, slug, title, is_homepage, published_at, active_until, deleted_at
		FROM landing_pages
		WHERE organization_id = $1 AND id = $2
		FOR UPDATE`
	markPaid := `UPDATE landing_page_activations
		SET status = 'paid', paid_at = $3
		WHERE organization_id = $1 AND id = $2 AND status IN ('open', 'void')`
	publish := `UPDATE landing_pages
		SET status = 'published', published_at = $3, active_until = $4, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND deleted_at IS NULL AND status <> 'archived'`

	var out ActivationSettlement
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		act, err := scanActivation(tx.QueryRow(ctx, lockActivation, scope.OrganizationID(), invoiceID))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		out.Found = true
		out.Activation = act
		if act.Status == domain.ActivationStatusPaid {
			return nil
		}

		var page domain.LandingPage
		err = tx.QueryRow(ctx, lockPage, scope.OrganizationID(), act.PageID).Scan(
			&page.ID, &page.OrganizationID, &page.Status, &page.Slug, &page.Title, &page.IsHomepage,
			&page.PublishedAt, &page.ActiveUntil, &page.DeletedAt)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return err
		default:
			out.PageFound = page.DeletedAt == nil
			out.PageSlug, out.PageTitle, out.IsHomepage = page.Slug, page.Title, page.IsHomepage
		}

		paidAt = paidAt.UTC()
		if _, err := tx.Exec(ctx, markPaid, scope.OrganizationID(), act.ID, paidAt); err != nil {
			return err
		}
		out.Applied = true
		act.Status, act.PaidAt = domain.ActivationStatusPaid, &paidAt
		out.Activation = act

		if !out.PageFound {
			return nil
		}
		s := domain.SettleActivationPayment(page, act, paidAt)
		if !s.Publish {
			return nil
		}
		if _, err := tx.Exec(ctx, publish, scope.OrganizationID(), act.PageID, s.PublishedAt, s.ActiveUntil); err != nil {
			return err
		}
		out.Published = true
		out.ActiveUntil = &s.ActiveUntil
		return nil
	})
	if err != nil {
		return ActivationSettlement{}, err
	}
	return out, nil
}

var _ ActivationSettler = (*activationRepository)(nil)

func (r *activationRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingPageActivation, error) {
	query := `SELECT ` + activationColumns + `
		FROM landing_page_activations
		WHERE organization_id = $1 AND id = $2`

	var a domain.LandingPageActivation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		a, err = scanActivation(tx.QueryRow(ctx, query, scope.OrganizationID(), id))
		return err
	})
	return a, err
}

// qualifiedActivationColumns prefixes activationColumns with a table alias, for joins where id,
// organization_id, status and created_at also exist on the other table.
func qualifiedActivationColumns(alias string) string {
	cols := strings.Split(strings.TrimSpace(activationColumns), ",")
	for i, c := range cols {
		cols[i] = alias + "." + strings.TrimSpace(c)
	}
	return strings.Join(cols, ", ")
}

func (r *activationRepository) ListByOrganization(ctx context.Context, scope coretenant.Scope, filter ActivationListFilter) ([]ActivationListItem, int64, error) {
	limit := filter.Limit
	if limit < 1 {
		limit = 20
	}
	offset := max(filter.Offset, 0)
	status := string(filter.Status)

	query := `SELECT ` + qualifiedActivationColumns("a") + `,
			p.title, p.slug, p.status, p.active_until
		FROM landing_page_activations a
		JOIN landing_pages p ON p.id = a.page_id AND p.organization_id = a.organization_id
		WHERE a.organization_id = $1 AND ($2 = '' OR a.status = $2)
		ORDER BY a.created_at DESC, a.id
		LIMIT $3 OFFSET $4`
	count := `SELECT count(*) FROM landing_page_activations a
		WHERE a.organization_id = $1 AND ($2 = '' OR a.status = $2)`

	var items []ActivationListItem
	var total int64
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, count, scope.OrganizationID(), status).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, query, scope.OrganizationID(), status, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var it ActivationListItem
			a := &it.Activation
			if err := rows.Scan(
				&a.ID, &a.OrganizationID, &a.PageID, &a.InvoiceID, &a.BuyerName, &a.BuyerContact, &a.BuyerEmail,
				&a.Price, &a.DurationValue, &a.DurationUnit, &a.PreviewTokenHash, &a.InvoiceDueAt,
				&a.Status, &a.PaidAt, &a.CreatedBy, &a.CreatedAt,
				&it.PageTitle, &it.PageSlug, &it.PageStatus, &it.PageActiveUntil,
			); err != nil {
				return err
			}
			items = append(items, it)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *activationRepository) RotatePreview(ctx context.Context, scope coretenant.Scope, id, tokenHash string, invoiceDueAt time.Time) (domain.LandingPageActivation, error) {
	query := `
		UPDATE landing_page_activations
		SET preview_token_hash = $3, invoice_due_at = $4
		WHERE organization_id = $1 AND id = $2 AND status = 'open'
		RETURNING ` + activationColumns

	var a domain.LandingPageActivation
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		a, err = scanActivation(tx.QueryRow(ctx, query, scope.OrganizationID(), id, tokenHash, invoiceDueAt.UTC()))
		return err
	})
	return a, err
}
