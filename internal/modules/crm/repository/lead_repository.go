package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/crm/domain"
	"zyad.cloud/internal/platform/database"
)

type leadRepository struct {
	db *database.Pool
}

func NewLeadRepository(db *database.Pool) LeadRepository {
	return &leadRepository{db: db}
}

func (r *leadRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

const leadColumns = `
	id, organization_id, contact_name, company_name, email, phone, source, status, score,
	owner_user_id, notes, converted_contact_id, converted_company_id, converted_deal_id, converted_at,
	created_by, updated_by, created_at, updated_at, deleted_at
`

func scanLead(row pgx.Row) (domain.Lead, error) {
	var l domain.Lead
	var companyName, email, phone, source, notes *string
	var ownerUserID, createdBy, updatedBy *string
	var status string

	err := row.Scan(
		&l.ID, &l.OrganizationID, &l.ContactName, &companyName, &email, &phone, &source, &status, &l.Score,
		&ownerUserID, &notes, &l.ConvertedContactID, &l.ConvertedCompanyID, &l.ConvertedDealID, &l.ConvertedAt,
		&createdBy, &updatedBy, &l.CreatedAt, &l.UpdatedAt, &l.DeletedAt,
	)
	if err != nil {
		return domain.Lead{}, err
	}

	l.Status = domain.LeadStatus(status)
	if companyName != nil {
		l.CompanyName = *companyName
	}
	if email != nil {
		l.Email = *email
	}
	if phone != nil {
		l.Phone = *phone
	}
	if source != nil {
		l.Source = *source
	}
	if notes != nil {
		l.Notes = *notes
	}
	if ownerUserID != nil {
		l.OwnerUserID = *ownerUserID
	}
	if createdBy != nil {
		l.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		l.UpdatedBy = *updatedBy
	}

	return l, nil
}

func (r *leadRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateLeadParams) (domain.Lead, error) {
	if !scope.IsValid() {
		return domain.Lead{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO crm_leads (
			organization_id, contact_name, company_name, email, phone, source, status, score,
			owner_user_id, notes, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, 'new', $7, $8, $9, $10
		) RETURNING ` + leadColumns

	var lead domain.Lead
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		lead, scanErr = scanLead(tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.ContactName,
			nullableString(params.CompanyName),
			nullableString(params.Email),
			nullableString(params.Phone),
			nullableString(params.Source),
			params.Score,
			nullableString(params.OwnerUserID),
			nullableString(params.Notes),
			nullableString(params.CreatedBy),
		))
		return scanErr
	})
	if err != nil {
		return domain.Lead{}, err
	}
	return lead, nil
}

func (r *leadRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Lead, error) {
	if !scope.IsValid() {
		return domain.Lead{}, coretenant.ErrInvalidScope
	}

	query := `SELECT ` + leadColumns + ` FROM crm_leads WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	var lead domain.Lead
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		lead, scanErr = scanLead(tx.QueryRow(ctx, query, id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Lead{}, err
	}
	return lead, nil
}

func (r *leadRepository) List(ctx context.Context, scope coretenant.Scope, filter LeadListFilter) ([]domain.Lead, int64, error) {
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
		idx := len(args)
		whereClauses = append(whereClauses, fmt.Sprintf("(contact_name ILIKE $%d OR company_name ILIKE $%d OR email ILIKE $%d)", idx, idx, idx))
	}
	if filter.Status != "" {
		args = append(args, string(filter.Status))
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", len(args)))
	}
	if filter.OwnerUserID != "" {
		args = append(args, filter.OwnerUserID)
		whereClauses = append(whereClauses, fmt.Sprintf("owner_user_id = $%d", len(args)))
	}

	where := strings.Join(whereClauses, " AND ")
	countQuery := "SELECT COUNT(*) FROM crm_leads WHERE " + where

	var leads []domain.Lead
	var total int64

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return err
		}
		if total == 0 {
			leads = []domain.Lead{}
			return nil
		}

		query := "SELECT " + leadColumns + " FROM crm_leads WHERE " + where + " ORDER BY created_at DESC"
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
			lead, scanErr := scanLead(rows)
			if scanErr != nil {
				return scanErr
			}
			leads = append(leads, lead)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}

	return leads, total, nil
}

func (r *leadRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateLeadParams) (domain.Lead, error) {
	if !scope.IsValid() {
		return domain.Lead{}, coretenant.ErrInvalidScope
	}

	setClauses := []string{"updated_at = NOW()"}
	var args []interface{}

	addSet := func(column string, value interface{}) {
		args = append(args, value)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if params.ContactName != nil {
		addSet("contact_name", *params.ContactName)
	}
	if params.CompanyName != nil {
		addSet("company_name", *params.CompanyName)
	}
	if params.Email != nil {
		addSet("email", *params.Email)
	}
	if params.Phone != nil {
		addSet("phone", *params.Phone)
	}
	if params.Source != nil {
		addSet("source", *params.Source)
	}
	if params.Status != nil {
		addSet("status", string(*params.Status))
	}
	if params.Score != nil {
		addSet("score", *params.Score)
	}
	if params.OwnerUserID != nil {
		addSet("owner_user_id", nullableString(*params.OwnerUserID))
	}
	if params.Notes != nil {
		addSet("notes", *params.Notes)
	}
	if params.UpdatedBy != "" {
		addSet("updated_by", params.UpdatedBy)
	}

	args = append(args, id, scope.OrganizationID())
	query := "UPDATE crm_leads SET " + strings.Join(setClauses, ", ") +
		fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", len(args)-1, len(args)) +
		leadColumns

	var lead domain.Lead
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		lead, scanErr = scanLead(tx.QueryRow(ctx, query, args...))
		return scanErr
	})
	if err != nil {
		return domain.Lead{}, err
	}
	return lead, nil
}

func (r *leadRepository) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_leads
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

func (r *leadRepository) Restore(ctx context.Context, scope coretenant.Scope, id string, restoredBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_leads
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

func (r *leadRepository) Assign(ctx context.Context, scope coretenant.Scope, id string, ownerUserID string, updatedBy string) (domain.Lead, error) {
	if !scope.IsValid() {
		return domain.Lead{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_leads
		SET owner_user_id = $1, updated_by = $2, updated_at = NOW()
		WHERE id = $3 AND organization_id = $4 AND deleted_at IS NULL
		RETURNING ` + leadColumns

	var lead domain.Lead
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		lead, scanErr = scanLead(tx.QueryRow(ctx, query, ownerUserID, nullableString(updatedBy), id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Lead{}, err
	}
	return lead, nil
}

func (r *leadRepository) MarkConverted(ctx context.Context, scope coretenant.Scope, id string, params MarkConvertedParams) (domain.Lead, error) {
	if !scope.IsValid() {
		return domain.Lead{}, coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_leads
		SET status = 'converted',
			converted_contact_id = $1,
			converted_company_id = $2,
			converted_at = NOW(),
			updated_by = $3,
			updated_at = NOW()
		WHERE id = $4 AND organization_id = $5 AND deleted_at IS NULL AND status <> 'converted'
		RETURNING ` + leadColumns

	var lead domain.Lead
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		lead, scanErr = scanLead(tx.QueryRow(ctx, query,
			params.ConvertedContactID,
			nullableString(params.ConvertedCompanyID),
			nullableString(params.UpdatedBy),
			id,
			scope.OrganizationID(),
		))
		return scanErr
	})
	if err != nil {
		return domain.Lead{}, err
	}
	return lead, nil
}
