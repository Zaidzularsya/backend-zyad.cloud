package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/receivable/domain"
	"zyad.cloud/internal/platform/database"
)

type accountRepository struct{ db *database.Pool }

func NewAccountRepository(db *database.Pool) AccountRepository { return &accountRepository{db: db} }

const accountColumns = `a.id, a.name, a.company_name, a.email, a.phone, a.address,
	COALESCE(a.source_type, ''), COALESCE(a.source_id::text, ''), a.created_at, a.updated_at`

func scanAccount(row pgx.Row) (domain.Account, error) {
	var a domain.Account
	err := row.Scan(&a.ID, &a.Name, &a.CompanyName, &a.Email, &a.Phone, &a.Address, &a.SourceType, &a.SourceID, &a.CreatedAt, &a.UpdatedAt)
	return a, err
}

func (r *accountRepository) Create(ctx context.Context, scope coretenant.Scope, p AccountParams, createdBy string) (domain.Account, error) {
	var out domain.Account
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			INSERT INTO receivable_accounts (organization_id, name, company_name, email, phone, address, source_type, source_id, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, name, company_name, email, phone, address,
				COALESCE(source_type, ''), COALESCE(source_id::text, ''), created_at, updated_at`,
			scope.OrganizationID(), p.Name, p.CompanyName, p.Email, p.Phone, p.Address,
			nullableString(p.SourceType), nullableString(p.SourceID), nullableString(createdBy))
		var err error
		out, err = scanAccount(row)
		return err
	})
	if isUniqueViolation(err, "idx_receivable_accounts_source_unique") {
		return domain.Account{}, ErrDuplicateAccountSource
	}
	return out, err
}

func (r *accountRepository) Update(ctx context.Context, scope coretenant.Scope, id string, p AccountParams) (domain.Account, error) {
	var out domain.Account
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE receivable_accounts a
			SET name = $3, company_name = $4, email = $5, phone = $6, address = $7, updated_at = now()
			WHERE a.id = $1 AND a.organization_id = $2 AND a.deleted_at IS NULL
			RETURNING `+accountColumns, id, scope.OrganizationID(), p.Name, p.CompanyName, p.Email, p.Phone, p.Address)
		var err error
		out, err = scanAccount(row)
		return err
	})
	return out, err
}

func (r *accountRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Account, error) {
	var out domain.Account
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = scanAccount(tx.QueryRow(ctx, `
			SELECT `+accountColumns+` FROM receivable_accounts a
			WHERE a.id = $1 AND a.organization_id = $2 AND a.deleted_at IS NULL`, id, scope.OrganizationID()))
		return err
	})
	return out, err
}

func (r *accountRepository) FindBySource(ctx context.Context, scope coretenant.Scope, sourceType, sourceID string) (domain.Account, error) {
	var out domain.Account
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		var err error
		out, err = scanAccount(tx.QueryRow(ctx, `
			SELECT `+accountColumns+` FROM receivable_accounts a
			WHERE a.organization_id = $1 AND a.source_type = $2 AND a.source_id = $3 AND a.deleted_at IS NULL`,
			scope.OrganizationID(), sourceType, sourceID))
		return err
	})
	return out, err
}

func (r *accountRepository) List(ctx context.Context, scope coretenant.Scope, search string, limit, offset int) ([]domain.Account, int64, error) {
	out := []domain.Account{}
	var total int64
	pattern := "%" + escapeLike(search) + "%"
	err := withScopedTx(ctx, r.db, scope, func(tx pgx.Tx) error {
		const where = `a.organization_id = $1 AND a.deleted_at IS NULL
			AND ($2 = '%%' OR a.name ILIKE $2 OR a.company_name ILIKE $2 OR a.email ILIKE $2)`
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM receivable_accounts a WHERE `+where, scope.OrganizationID(), pattern).Scan(&total); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT `+accountColumns+` FROM receivable_accounts a WHERE `+where+`
			ORDER BY lower(a.name), a.id LIMIT $3 OFFSET $4`, scope.OrganizationID(), pattern, clampLimit(limit), max(offset, 0))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, err := scanAccount(rows)
			if err != nil {
				return err
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, total, err
}
