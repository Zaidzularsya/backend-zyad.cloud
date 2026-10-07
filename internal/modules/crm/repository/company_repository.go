package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/platform/database"
)

type companyRepository struct {
	db *database.Pool
}

func NewCompanyRepository(db *database.Pool) CompanyRepository {
	return &companyRepository{db: db}
}

func (r *companyRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, "SELECT set_config('app.organization_id', $1, true)", scope.OrganizationID())
	if err != nil {
		return err
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

const companyColumns = `
	id, organization_id, name, industry, website, phone, email, address,
	size_range, notes, tags, owner_user_id, created_by, updated_by,
	created_at, updated_at, deleted_at, tenant_organization_id
`

func scanCompany(row pgx.Row) (domain.Company, error) {
	var c domain.Company
	var industry, website, phone, email, sizeRange, notes *string
	var ownerUserID, createdBy, updatedBy *string

	err := row.Scan(
		&c.ID, &c.OrganizationID, &c.Name, &industry, &website, &phone, &email, &c.Address,
		&sizeRange, &notes, &c.Tags, &ownerUserID, &createdBy, &updatedBy,
		&c.CreatedAt, &c.UpdatedAt, &c.DeletedAt, &c.TenantOrganizationID,
	)
	if err != nil {
		return domain.Company{}, err
	}

	if industry != nil {
		c.Industry = *industry
	}
	if website != nil {
		c.Website = *website
	}
	if phone != nil {
		c.Phone = *phone
	}
	if email != nil {
		c.Email = *email
	}
	if sizeRange != nil {
		c.SizeRange = *sizeRange
	}
	if notes != nil {
		c.Notes = *notes
	}
	if ownerUserID != nil {
		c.OwnerUserID = *ownerUserID
	}
	if createdBy != nil {
		c.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		c.UpdatedBy = *updatedBy
	}

	return c, nil
}

func (r *companyRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateCompanyParams) (domain.Company, error) {
	if !scope.IsValid() {
		return domain.Company{}, coretenant.ErrInvalidScope
	}
	var company domain.Company
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		company, err = insertCompanyTx(ctx, tx, scope, params)
		return err
	})
	if err != nil {
		return domain.Company{}, err
	}
	return company, nil
}

// insertCompanyTx menulis satu crm_companies di transaksi pemanggil
// (dipakai Create dan convert lead).
func insertCompanyTx(ctx context.Context, tx pgx.Tx, scope coretenant.Scope, params CreateCompanyParams) (domain.Company, error) {
	address := params.Address
	if address == nil {
		address = map[string]any{}
	}
	tags := params.Tags
	if tags == nil {
		tags = []string{}
	}
	return scanCompany(tx.QueryRow(ctx, `
		INSERT INTO crm_companies (
			organization_id, name, industry, website, phone, email, address,
			size_range, notes, tags, owner_user_id, created_by, tenant_organization_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		) RETURNING `+companyColumns,
		scope.OrganizationID(),
		params.Name,
		nullableString(params.Industry),
		nullableString(params.Website),
		nullableString(params.Phone),
		nullableString(params.Email),
		address,
		nullableString(params.SizeRange),
		nullableString(params.Notes),
		tags,
		nullableString(params.OwnerUserID),
		nullableString(params.CreatedBy),
		nullableString(params.TenantOrganizationID),
	))
}

func (r *companyRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Company, error) {
	if !scope.IsValid() {
		return domain.Company{}, coretenant.ErrInvalidScope
	}

	query := `SELECT ` + companyColumns + ` FROM crm_companies WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	var company domain.Company
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		company, scanErr = scanCompany(tx.QueryRow(ctx, query, id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Company{}, err
	}
	return company, nil
}

func (r *companyRepository) List(ctx context.Context, scope coretenant.Scope, filter CompanyListFilter) ([]domain.Company, int64, error) {
	if !scope.IsValid() {
		return nil, 0, coretenant.ErrInvalidScope
	}

	whereClauses := []string{"organization_id = $1"}
	args := []interface{}{scope.OrganizationID()}

	if !filter.IncludeDeleted {
		whereClauses = append(whereClauses, "deleted_at IS NULL")
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		whereClauses = append(whereClauses, fmt.Sprintf("name ILIKE $%d", len(args)))
	}
	if filter.OwnerUserID != "" {
		args = append(args, filter.OwnerUserID)
		whereClauses = append(whereClauses, fmt.Sprintf("owner_user_id = $%d", len(args)))
	}

	where := strings.Join(whereClauses, " AND ")
	countQuery := "SELECT COUNT(*) FROM crm_companies WHERE " + where

	var companies []domain.Company
	var total int64

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return err
		}
		if total == 0 {
			companies = []domain.Company{}
			return nil
		}

		query := "SELECT " + companyColumns + " FROM crm_companies WHERE " + where + " ORDER BY created_at DESC"
		queryArgs := append([]interface{}{}, args...)
		if filter.Limit > 0 {
			queryArgs = append(queryArgs, filter.Limit)
			query += fmt.Sprintf(" LIMIT $%d", len(queryArgs))
		}
		if filter.Offset > 0 {
			queryArgs = append(queryArgs, filter.Offset)
			query += fmt.Sprintf(" OFFSET $%d", len(queryArgs))
		}

		rows, err := tx.Query(ctx, query, queryArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			company, scanErr := scanCompany(rows)
			if scanErr != nil {
				return scanErr
			}
			companies = append(companies, company)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}

	return companies, total, nil
}

func (r *companyRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateCompanyParams) (domain.Company, error) {
	if !scope.IsValid() {
		return domain.Company{}, coretenant.ErrInvalidScope
	}

	setClauses := []string{"updated_at = NOW()"}
	var args []interface{}

	addSet := func(column string, value interface{}) {
		args = append(args, value)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if params.Name != nil {
		addSet("name", *params.Name)
	}
	if params.Industry != nil {
		addSet("industry", *params.Industry)
	}
	if params.Website != nil {
		addSet("website", *params.Website)
	}
	if params.Phone != nil {
		addSet("phone", *params.Phone)
	}
	if params.Email != nil {
		addSet("email", *params.Email)
	}
	if params.Address != nil {
		addSet("address", params.Address)
	}
	if params.SizeRange != nil {
		addSet("size_range", *params.SizeRange)
	}
	if params.Notes != nil {
		addSet("notes", *params.Notes)
	}
	if params.Tags != nil {
		addSet("tags", params.Tags)
	}
	if params.OwnerUserID != nil {
		addSet("owner_user_id", *params.OwnerUserID)
	}
	if params.UpdatedBy != "" {
		addSet("updated_by", params.UpdatedBy)
	}

	args = append(args, id, scope.OrganizationID())
	query := "UPDATE crm_companies SET " + strings.Join(setClauses, ", ") +
		fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", len(args)-1, len(args)) +
		companyColumns

	var company domain.Company
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		company, scanErr = scanCompany(tx.QueryRow(ctx, query, args...))
		return scanErr
	})
	if err != nil {
		return domain.Company{}, err
	}
	return company, nil
}

func (r *companyRepository) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_companies
		SET deleted_at = NOW(), updated_by = $1
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, nullableString(deletedBy), id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *companyRepository) Restore(ctx context.Context, scope coretenant.Scope, id string, restoredBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_companies
		SET deleted_at = NULL, updated_by = $1
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NOT NULL
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, nullableString(restoredBy), id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func nullableString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func (r *companyRepository) FindCandidatesByName(ctx context.Context, scope coretenant.Scope, needle string, limit int) ([]domain.Company, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}
	escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(needle)
	var out []domain.Company
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT "+companyColumns+` FROM crm_companies
			WHERE organization_id = $1 AND deleted_at IS NULL AND lower(name) LIKE '%' || $2 || '%' ESCAPE '\'
			ORDER BY name LIMIT $3`, scope.OrganizationID(), escaped, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanCompany(rows)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

func (r *companyRepository) FindByTenantOrganization(ctx context.Context, scope coretenant.Scope, tenantOrgID string) (domain.Company, error) {
	if !scope.IsValid() {
		return domain.Company{}, coretenant.ErrInvalidScope
	}
	var company domain.Company
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		company, err = scanCompany(tx.QueryRow(ctx, `SELECT `+companyColumns+` FROM crm_companies
			WHERE organization_id = $1 AND tenant_organization_id = $2 AND deleted_at IS NULL`,
			scope.OrganizationID(), tenantOrgID))
		return err
	})
	if err != nil {
		return domain.Company{}, err
	}
	return company, nil
}

func (r *companyRepository) SetTenantOrganization(ctx context.Context, scope coretenant.Scope, companyID, tenantOrgID, updatedBy string) (domain.Company, error) {
	if !scope.IsValid() {
		return domain.Company{}, coretenant.ErrInvalidScope
	}
	var company domain.Company
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var err error
		company, err = scanCompany(tx.QueryRow(ctx, `UPDATE crm_companies
			SET tenant_organization_id = NULLIF($3::text, '')::uuid, updated_by = $4, updated_at = NOW()
			WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
			RETURNING `+companyColumns, companyID, scope.OrganizationID(), tenantOrgID, nullableString(updatedBy)))
		return err
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_crm_companies_tenant_org_unique" {
		return domain.Company{}, ErrWorkspaceAlreadyLinked
	}
	if err != nil {
		return domain.Company{}, err
	}
	return company, nil
}
