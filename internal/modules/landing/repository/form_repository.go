package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

type formRepository struct {
	db *database.Pool
}

func NewFormRepository(db *database.Pool) FormRepository {
	return &formRepository{db: db}
}

// withTx executes a function within a tenant-scoped transaction
func (r *formRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

func (r *formRepository) Create(ctx context.Context, scope coretenant.Scope, params CreateFormParams) (domain.LandingForm, error) {
	if !scope.IsValid() {
		return domain.LandingForm{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_forms (
			organization_id, landing_page_id, name, key, description,
			submit_label, success_message, redirect_url, is_active, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		) RETURNING
			id, landing_page_id, name, key, description,
			submit_label, success_message, redirect_url, is_active,
			created_at, updated_at
	`

	var form domain.LandingForm
	var createdBy interface{} = nil
	if params.CreatedBy != "" {
		createdBy = params.CreatedBy
	}

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.LandingPageID,
			params.Name,
			params.Key,
			params.Description,
			params.SubmitLabel,
			params.SuccessMessage,
			params.RedirectURL,
			params.IsActive,
			createdBy,
		).Scan(
			&form.ID, &form.LandingPageID, &form.Name, &form.Key, &form.Description,
			&form.SubmitLabel, &form.SuccessMessage, &form.RedirectURL, &form.IsActive,
			&form.CreatedAt, &form.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingForm{}, err
	}

	return form, nil
}

func (r *formRepository) FindByID(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingForm, error) {
	if !scope.IsValid() {
		return domain.LandingForm{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, name, key, description,
			submit_label, success_message, redirect_url, is_active,
			created_at, updated_at
		FROM landing_forms
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`

	var form domain.LandingForm

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&form.ID, &form.LandingPageID, &form.Name, &form.Key, &form.Description,
			&form.SubmitLabel, &form.SuccessMessage, &form.RedirectURL, &form.IsActive,
			&form.CreatedAt, &form.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingForm{}, err
	}

	return form, nil
}

func (r *formRepository) FindByKey(ctx context.Context, scope coretenant.Scope, pageID string, key string) (domain.LandingForm, error) {
	if !scope.IsValid() {
		return domain.LandingForm{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, name, key, description,
			submit_label, success_message, redirect_url, is_active,
			created_at, updated_at
		FROM landing_forms
		WHERE landing_page_id = $1 AND lower(key) = lower($2) AND organization_id = $3 AND deleted_at IS NULL
	`

	var form domain.LandingForm

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, pageID, key, scope.OrganizationID()).Scan(
			&form.ID, &form.LandingPageID, &form.Name, &form.Key, &form.Description,
			&form.SubmitLabel, &form.SuccessMessage, &form.RedirectURL, &form.IsActive,
			&form.CreatedAt, &form.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingForm{}, err
	}

	return form, nil
}

func (r *formRepository) ListByPage(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingForm, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, name, key, description,
			submit_label, success_message, redirect_url, is_active,
			created_at, updated_at
		FROM landing_forms
		WHERE landing_page_id = $1 AND organization_id = $2 AND deleted_at IS NULL
		ORDER BY created_at ASC
	`

	var forms []domain.LandingForm

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, pageID, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var form domain.LandingForm
			err := rows.Scan(
				&form.ID, &form.LandingPageID, &form.Name, &form.Key, &form.Description,
				&form.SubmitLabel, &form.SuccessMessage, &form.RedirectURL, &form.IsActive,
				&form.CreatedAt, &form.UpdatedAt,
			)
			if err != nil {
				return err
			}
			forms = append(forms, form)
		}

		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return forms, nil
}

func (r *formRepository) Update(ctx context.Context, scope coretenant.Scope, id string, params UpdateFormParams) (domain.LandingForm, error) {
	if !scope.IsValid() {
		return domain.LandingForm{}, coretenant.ErrInvalidScope
	}

	query := `UPDATE landing_forms SET updated_at = NOW()`
	var args []interface{}
	argCount := 1

	if params.Name != nil {
		args = append(args, *params.Name)
		query += fmt.Sprintf(", name = $%d", argCount)
		argCount++
	}
	if params.Description != nil {
		args = append(args, *params.Description)
		query += fmt.Sprintf(", description = $%d", argCount)
		argCount++
	}
	if params.SubmitLabel != nil {
		args = append(args, *params.SubmitLabel)
		query += fmt.Sprintf(", submit_label = $%d", argCount)
		argCount++
	}
	if params.SuccessMessage != nil {
		args = append(args, *params.SuccessMessage)
		query += fmt.Sprintf(", success_message = $%d", argCount)
		argCount++
	}
	if params.RedirectURL != nil {
		args = append(args, *params.RedirectURL)
		query += fmt.Sprintf(", redirect_url = $%d", argCount)
		argCount++
	}
	if params.IsActive != nil {
		args = append(args, *params.IsActive)
		query += fmt.Sprintf(", is_active = $%d", argCount)
		argCount++
	}
	if params.UpdatedBy != "" {
		args = append(args, params.UpdatedBy)
		query += fmt.Sprintf(", updated_by = $%d", argCount)
		argCount++
	}

	args = append(args, id, scope.OrganizationID())
	query += fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", argCount, argCount+1)
	query += `
		id, landing_page_id, name, key, description,
		submit_label, success_message, redirect_url, is_active,
		created_at, updated_at
	`

	var form domain.LandingForm

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(
			&form.ID, &form.LandingPageID, &form.Name, &form.Key, &form.Description,
			&form.SubmitLabel, &form.SuccessMessage, &form.RedirectURL, &form.IsActive,
			&form.CreatedAt, &form.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingForm{}, err
	}

	return form, nil
}

func (r *formRepository) Delete(ctx context.Context, scope coretenant.Scope, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE landing_forms
		SET deleted_at = NOW()
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})

	return err
}

func (r *formRepository) CreateField(ctx context.Context, scope coretenant.Scope, params CreateFormFieldParams) (domain.LandingFormField, error) {
	if !scope.IsValid() {
		return domain.LandingFormField{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_form_fields (
			organization_id, form_id, field_key, field_type, label,
			placeholder, options, validation, is_required, sort_order
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		) RETURNING
			id, form_id, field_key, field_type, label,
			placeholder, options, validation, is_required, sort_order,
			created_at, updated_at
	`

	var field domain.LandingFormField

	options := params.Options
	if options == nil {
		options = []string{}
	}
	validation := params.Validation
	if validation == nil {
		validation = map[string]any{}
	}

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.FormID,
			params.Key,
			params.Type,
			params.Label,
			params.Placeholder,
			options,
			validation,
			params.IsRequired,
			params.SortOrder,
		).Scan(
			&field.ID, &field.FormID, &field.Key, &field.Type, &field.Label,
			&field.Placeholder, &field.Options, &field.Validation, &field.IsRequired, &field.SortOrder,
			&field.CreatedAt, &field.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingFormField{}, err
	}

	return field, nil
}

func (r *formRepository) FindFieldByID(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingFormField, error) {
	if !scope.IsValid() {
		return domain.LandingFormField{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, form_id, field_key, field_type, label,
			placeholder, options, validation, is_required, sort_order,
			created_at, updated_at
		FROM landing_form_fields
		WHERE id = $1 AND organization_id = $2
	`

	var field domain.LandingFormField

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&field.ID, &field.FormID, &field.Key, &field.Type, &field.Label,
			&field.Placeholder, &field.Options, &field.Validation, &field.IsRequired, &field.SortOrder,
			&field.CreatedAt, &field.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingFormField{}, err
	}

	return field, nil
}

func (r *formRepository) ListFieldsByForm(ctx context.Context, scope coretenant.Scope, formID string) ([]domain.LandingFormField, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, form_id, field_key, field_type, label,
			placeholder, options, validation, is_required, sort_order,
			created_at, updated_at
		FROM landing_form_fields
		WHERE form_id = $1 AND organization_id = $2
		ORDER BY sort_order ASC
	`

	var fields []domain.LandingFormField

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, formID, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var field domain.LandingFormField
			err := rows.Scan(
				&field.ID, &field.FormID, &field.Key, &field.Type, &field.Label,
				&field.Placeholder, &field.Options, &field.Validation, &field.IsRequired, &field.SortOrder,
				&field.CreatedAt, &field.UpdatedAt,
			)
			if err != nil {
				return err
			}
			fields = append(fields, field)
		}

		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return fields, nil
}

func (r *formRepository) UpdateField(ctx context.Context, scope coretenant.Scope, id string, params UpdateFormFieldParams) (domain.LandingFormField, error) {
	if !scope.IsValid() {
		return domain.LandingFormField{}, coretenant.ErrInvalidScope
	}

	query := `UPDATE landing_form_fields SET updated_at = NOW()`
	var args []interface{}
	argCount := 1

	if params.Label != nil {
		args = append(args, *params.Label)
		query += fmt.Sprintf(", label = $%d", argCount)
		argCount++
	}
	if params.Placeholder != nil {
		args = append(args, *params.Placeholder)
		query += fmt.Sprintf(", placeholder = $%d", argCount)
		argCount++
	}
	if params.Options != nil {
		args = append(args, params.Options)
		query += fmt.Sprintf(", options = $%d", argCount)
		argCount++
	}
	if params.Validation != nil {
		args = append(args, params.Validation)
		query += fmt.Sprintf(", validation = $%d", argCount)
		argCount++
	}
	if params.IsRequired != nil {
		args = append(args, *params.IsRequired)
		query += fmt.Sprintf(", is_required = $%d", argCount)
		argCount++
	}

	args = append(args, id, scope.OrganizationID())
	query += fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d RETURNING ", argCount, argCount+1)
	query += `
		id, form_id, field_key, field_type, label,
		placeholder, options, validation, is_required, sort_order,
		created_at, updated_at
	`

	var field domain.LandingFormField

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(
			&field.ID, &field.FormID, &field.Key, &field.Type, &field.Label,
			&field.Placeholder, &field.Options, &field.Validation, &field.IsRequired, &field.SortOrder,
			&field.CreatedAt, &field.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingFormField{}, err
	}

	return field, nil
}

func (r *formRepository) ReorderFields(ctx context.Context, scope coretenant.Scope, formID string, params []FormFieldReorderParam) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		for _, p := range params {
			query1 := `
				UPDATE landing_form_fields
				SET sort_order = $1 + 1000000
				WHERE id = $2 AND form_id = $3 AND organization_id = $4
			`
			_, err := tx.Exec(ctx, query1, p.SortOrder, p.ID, formID, scope.OrganizationID())
			if err != nil {
				return err
			}
		}

		for _, p := range params {
			query2 := `
				UPDATE landing_form_fields
				SET sort_order = $1
				WHERE id = $2 AND form_id = $3 AND organization_id = $4
			`
			_, err := tx.Exec(ctx, query2, p.SortOrder, p.ID, formID, scope.OrganizationID())
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *formRepository) DeleteField(ctx context.Context, scope coretenant.Scope, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		DELETE FROM landing_form_fields
		WHERE id = $1 AND organization_id = $2
	`

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})

	return err
}
