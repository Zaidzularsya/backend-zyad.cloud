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

type contactRepository struct {
	db *database.Pool
}

func NewContactRepository(db *database.Pool) ContactRepository {
	return &contactRepository{db: db}
}

func (r *contactRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

const contactColumns = `
	id, organization_id, company_id, first_name, last_name, email, phone, job_title,
	address, tags, source, owner_user_id, is_customer, lifecycle_stage,
	created_by, updated_by, created_at, updated_at, deleted_at
`

func scanContact(row pgx.Row) (domain.Contact, error) {
	var c domain.Contact
	var companyID *string
	var lastName, email, phone, jobTitle, source *string
	var ownerUserID, createdBy, updatedBy *string
	var lifecycleStage string

	err := row.Scan(
		&c.ID, &c.OrganizationID, &companyID, &c.FirstName, &lastName, &email, &phone, &jobTitle,
		&c.Address, &c.Tags, &source, &ownerUserID, &c.IsCustomer, &lifecycleStage,
		&createdBy, &updatedBy, &c.CreatedAt, &c.UpdatedAt, &c.DeletedAt,
	)
	if err != nil {
		return domain.Contact{}, err
	}

	c.CompanyID = companyID
	c.LifecycleStage = domain.ContactLifecycleStage(lifecycleStage)
	if lastName != nil {
		c.LastName = *lastName
	}
	if email != nil {
		c.Email = *email
	}
	if phone != nil {
		c.Phone = *phone
	}
	if jobTitle != nil {
		c.JobTitle = *jobTitle
	}
	if source != nil {
		c.Source = *source
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

func (r *contactRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateContactParams) (domain.Contact, error) {
	if !scope.IsValid() {
		return domain.Contact{}, coretenant.ErrInvalidScope
	}

	lifecycleStage := params.LifecycleStage
	if lifecycleStage == "" {
		lifecycleStage = domain.ContactLifecycleContact
	}
	address := params.Address
	if address == nil {
		address = map[string]any{}
	}
	tags := params.Tags
	if tags == nil {
		tags = []string{}
	}

	query := `
		INSERT INTO crm_contacts (
			organization_id, company_id, first_name, last_name, email, phone, job_title,
			address, tags, source, owner_user_id, is_customer, lifecycle_stage, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		) RETURNING ` + contactColumns

	var contact domain.Contact
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		contact, scanErr = scanContact(tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			nullableString(params.CompanyID),
			params.FirstName,
			nullableString(params.LastName),
			nullableString(params.Email),
			nullableString(params.Phone),
			nullableString(params.JobTitle),
			address,
			tags,
			nullableString(params.Source),
			nullableString(params.OwnerUserID),
			params.IsCustomer,
			string(lifecycleStage),
			nullableString(params.CreatedBy),
		))
		return scanErr
	})
	if err != nil {
		return domain.Contact{}, err
	}
	return contact, nil
}

func (r *contactRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.Contact, error) {
	if !scope.IsValid() {
		return domain.Contact{}, coretenant.ErrInvalidScope
	}

	query := `SELECT ` + contactColumns + ` FROM crm_contacts WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL`

	var contact domain.Contact
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		contact, scanErr = scanContact(tx.QueryRow(ctx, query, id, scope.OrganizationID()))
		return scanErr
	})
	if err != nil {
		return domain.Contact{}, err
	}
	return contact, nil
}

func buildContactWhere(scope coretenant.Scope, filter ContactListFilter) (string, []interface{}) {
	whereClauses := []string{"organization_id = $1"}
	args := []interface{}{scope.OrganizationID()}

	if !filter.IncludeDeleted {
		whereClauses = append(whereClauses, "deleted_at IS NULL")
	}
	if filter.Search != "" {
		args = append(args, "%"+filter.Search+"%")
		idx := len(args)
		whereClauses = append(whereClauses, fmt.Sprintf("(first_name ILIKE $%d OR last_name ILIKE $%d OR email ILIKE $%d)", idx, idx, idx))
	}
	if filter.CompanyID != "" {
		args = append(args, filter.CompanyID)
		whereClauses = append(whereClauses, fmt.Sprintf("company_id = $%d", len(args)))
	}
	if filter.OwnerUserID != "" {
		args = append(args, filter.OwnerUserID)
		whereClauses = append(whereClauses, fmt.Sprintf("owner_user_id = $%d", len(args)))
	}
	if filter.LifecycleStage != "" {
		args = append(args, string(filter.LifecycleStage))
		whereClauses = append(whereClauses, fmt.Sprintf("lifecycle_stage = $%d", len(args)))
	}
	if filter.IsCustomer != nil {
		args = append(args, *filter.IsCustomer)
		whereClauses = append(whereClauses, fmt.Sprintf("is_customer = $%d", len(args)))
	}

	return strings.Join(whereClauses, " AND "), args
}

func (r *contactRepository) List(ctx context.Context, scope coretenant.Scope, filter ContactListFilter) ([]domain.Contact, int64, error) {
	if !scope.IsValid() {
		return nil, 0, coretenant.ErrInvalidScope
	}

	where, args := buildContactWhere(scope, filter)
	countQuery := "SELECT COUNT(*) FROM crm_contacts WHERE " + where

	var contacts []domain.Contact
	var total int64

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
			return err
		}
		if total == 0 {
			contacts = []domain.Contact{}
			return nil
		}

		query := "SELECT " + contactColumns + " FROM crm_contacts WHERE " + where + " ORDER BY created_at DESC"
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
			contact, scanErr := scanContact(rows)
			if scanErr != nil {
				return scanErr
			}
			contacts = append(contacts, contact)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, 0, err
	}

	return contacts, total, nil
}

func (r *contactRepository) Count(ctx context.Context, scope coretenant.Scope, filter ContactListFilter) (int64, error) {
	if !scope.IsValid() {
		return 0, coretenant.ErrInvalidScope
	}

	where, args := buildContactWhere(scope, filter)
	countQuery := "SELECT COUNT(*) FROM crm_contacts WHERE " + where

	var total int64
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, countQuery, args...).Scan(&total)
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

func (r *contactRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateContactParams) (domain.Contact, error) {
	if !scope.IsValid() {
		return domain.Contact{}, coretenant.ErrInvalidScope
	}

	setClauses := []string{"updated_at = NOW()"}
	var args []interface{}

	addSet := func(column string, value interface{}) {
		args = append(args, value)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if params.CompanyID != nil {
		addSet("company_id", nullableString(*params.CompanyID))
	}
	if params.FirstName != nil {
		addSet("first_name", *params.FirstName)
	}
	if params.LastName != nil {
		addSet("last_name", *params.LastName)
	}
	if params.Email != nil {
		addSet("email", *params.Email)
	}
	if params.Phone != nil {
		addSet("phone", *params.Phone)
	}
	if params.JobTitle != nil {
		addSet("job_title", *params.JobTitle)
	}
	if params.Address != nil {
		addSet("address", params.Address)
	}
	if params.Tags != nil {
		addSet("tags", params.Tags)
	}
	if params.Source != nil {
		addSet("source", *params.Source)
	}
	if params.OwnerUserID != nil {
		addSet("owner_user_id", nullableString(*params.OwnerUserID))
	}
	if params.IsCustomer != nil {
		addSet("is_customer", *params.IsCustomer)
	}
	if params.LifecycleStage != nil {
		addSet("lifecycle_stage", string(*params.LifecycleStage))
	}
	if params.UpdatedBy != "" {
		addSet("updated_by", params.UpdatedBy)
	}

	args = append(args, id, scope.OrganizationID())
	query := "UPDATE crm_contacts SET " + strings.Join(setClauses, ", ") +
		fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", len(args)-1, len(args)) +
		contactColumns

	var contact domain.Contact
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var scanErr error
		contact, scanErr = scanContact(tx.QueryRow(ctx, query, args...))
		return scanErr
	})
	if err != nil {
		return domain.Contact{}, err
	}
	return contact, nil
}

func (r *contactRepository) Delete(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_contacts
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

func (r *contactRepository) Restore(ctx context.Context, scope coretenant.Scope, id string, restoredBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE crm_contacts
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
