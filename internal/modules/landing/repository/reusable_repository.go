package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

type reusableRepository struct {
	db *database.Pool
}

func NewReusableRepository(db *database.Pool) ReusableRepository {
	return &reusableRepository{db: db}
}

func (r *reusableRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

func (r *reusableRepository) CreateSectionTemplate(ctx context.Context, scope coretenant.Scope, params CreateSectionTemplateParams) (domain.SectionTemplate, error) {
	if !scope.IsValid() {
		return domain.SectionTemplate{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_section_templates (
			organization_id, name, description, section_type, content, style, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		) RETURNING
			id, name, description, section_type, content, style, created_by, updated_by, created_at, updated_at
	`

	var t domain.SectionTemplate
	var createdBy, updatedBy *string
	var desc *string

	if params.Description != "" {
		desc = &params.Description
	}

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var cb interface{} = nil
		if params.CreatedBy != "" {
			cb = params.CreatedBy
		}

		err := tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.Name,
			desc,
			params.SectionType,
			params.Content,
			params.Style,
			cb,
		).Scan(
			&t.ID, &t.Name, &desc, &t.SectionType, &t.Content, &t.Style,
			&createdBy, &updatedBy, &t.CreatedAt, &t.UpdatedAt,
		)
		return err
	})

	if err != nil {
		return domain.SectionTemplate{}, err
	}
	t.OrganizationID = scope.OrganizationID()
	if desc != nil {
		t.Description = *desc
	}
	if createdBy != nil {
		t.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		t.UpdatedBy = *updatedBy
	}

	return t, nil
}

func (r *reusableRepository) GetSectionTemplate(ctx context.Context, scope coretenant.Scope, id string) (domain.SectionTemplate, error) {
	if !scope.IsValid() {
		return domain.SectionTemplate{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, organization_id, name, description, section_type, content, style, created_by, updated_by, created_at, updated_at
		FROM landing_section_templates
		WHERE id = $1
			AND deleted_at IS NULL
			AND (
				organization_id = $2
				OR organization_id = (
					SELECT id FROM organizations
					WHERE type = 'platform'
					ORDER BY created_at ASC
					LIMIT 1
				)
			)
	`

	var t domain.SectionTemplate
	var createdBy, updatedBy *string
	var desc *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&t.ID, &t.OrganizationID, &t.Name, &desc, &t.SectionType, &t.Content, &t.Style,
			&createdBy, &updatedBy, &t.CreatedAt, &t.UpdatedAt,
		)
	})

	if err != nil {
		return domain.SectionTemplate{}, err
	}
	if desc != nil {
		t.Description = *desc
	}
	if createdBy != nil {
		t.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		t.UpdatedBy = *updatedBy
	}

	return t, nil
}

func (r *reusableRepository) ListSectionTemplates(ctx context.Context, scope coretenant.Scope, sType domain.SectionType) ([]domain.SectionTemplate, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, organization_id, name, description, section_type, content, style, created_by, updated_by, created_at, updated_at
		FROM landing_section_templates
		WHERE deleted_at IS NULL
			AND (
				organization_id = $1
				OR organization_id = (
					SELECT id FROM organizations
					WHERE type = 'platform'
					ORDER BY created_at ASC
					LIMIT 1
				)
			)
	`
	args := []interface{}{scope.OrganizationID()}

	if sType != "" {
		query += ` AND section_type = $2`
		args = append(args, sType)
	}

	query += ` ORDER BY created_at DESC`

	var templates []domain.SectionTemplate

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var t domain.SectionTemplate
			var createdBy, updatedBy *string
			var desc *string

			err := rows.Scan(
				&t.ID, &t.OrganizationID, &t.Name, &desc, &t.SectionType, &t.Content, &t.Style,
				&createdBy, &updatedBy, &t.CreatedAt, &t.UpdatedAt,
			)
			if err != nil {
				return err
			}
			if desc != nil {
				t.Description = *desc
			}
			if createdBy != nil {
				t.CreatedBy = *createdBy
			}
			if updatedBy != nil {
				t.UpdatedBy = *updatedBy
			}
			templates = append(templates, t)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return templates, nil
}

func (r *reusableRepository) UpdateSectionTemplate(ctx context.Context, scope coretenant.Scope, id string, params UpdateSectionTemplateParams) (domain.SectionTemplate, error) {
	if !scope.IsValid() {
		return domain.SectionTemplate{}, coretenant.ErrInvalidScope
	}

	query := `UPDATE landing_section_templates SET updated_at = NOW()`
	var args []interface{}
	argCount := 1

	if params.Name != nil {
		args = append(args, *params.Name)
		query += fmt.Sprintf(", name = $%d", argCount)
		argCount++
	}
	if params.Description != nil {
		if *params.Description == "" {
			query += ", description = NULL"
		} else {
			args = append(args, *params.Description)
			query += fmt.Sprintf(", description = $%d", argCount)
			argCount++
		}
	}
	if params.Content != nil {
		args = append(args, params.Content)
		query += fmt.Sprintf(", content = $%d", argCount)
		argCount++
	}
	if params.Style != nil {
		args = append(args, params.Style)
		query += fmt.Sprintf(", style = $%d", argCount)
		argCount++
	}
	if params.UpdatedBy != "" {
		args = append(args, params.UpdatedBy)
		query += fmt.Sprintf(", updated_by = $%d", argCount)
		argCount++
	}

	args = append(args, id, scope.OrganizationID())
	query += fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", argCount, argCount+1)
	query += `id, name, description, section_type, content, style, created_by, updated_by, created_at, updated_at`

	var t domain.SectionTemplate
	var createdBy, updatedBy *string
	var desc *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(
			&t.ID, &t.Name, &desc, &t.SectionType, &t.Content, &t.Style,
			&createdBy, &updatedBy, &t.CreatedAt, &t.UpdatedAt,
		)
	})

	if err != nil {
		return domain.SectionTemplate{}, err
	}
	t.OrganizationID = scope.OrganizationID()
	if desc != nil {
		t.Description = *desc
	}
	if createdBy != nil {
		t.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		t.UpdatedBy = *updatedBy
	}

	return t, nil
}

func (r *reusableRepository) DeleteSectionTemplate(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE landing_section_templates
		SET deleted_at = NOW(), updated_by = $1
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var ub interface{} = nil
		if deletedBy != "" {
			ub = deletedBy
		}
		cmdTag, err := tx.Exec(ctx, query, ub, id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *reusableRepository) CreateCTA(ctx context.Context, scope coretenant.Scope, params CreateCTAParams) (domain.LandingCTA, error) {
	if !scope.IsValid() {
		return domain.LandingCTA{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_ctas (
			organization_id, name, label, type, target, destination, tracking_key, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		) RETURNING
			id, name, label, type, target, destination, tracking_key, created_by, updated_by, created_at, updated_at
	`

	var c domain.LandingCTA
	var createdBy, updatedBy *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var cb interface{} = nil
		if params.CreatedBy != "" {
			cb = params.CreatedBy
		}

		err := tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.Name,
			params.Label,
			params.Type,
			params.Target,
			params.Destination,
			params.TrackingKey,
			cb,
		).Scan(
			&c.ID, &c.Name, &c.Label, &c.Type, &c.Target, &c.Destination, &c.TrackingKey,
			&createdBy, &updatedBy, &c.CreatedAt, &c.UpdatedAt,
		)
		return err
	})

	if err != nil {
		return domain.LandingCTA{}, err
	}
	c.OrganizationID = scope.OrganizationID()
	if createdBy != nil {
		c.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		c.UpdatedBy = *updatedBy
	}

	return c, nil
}

func (r *reusableRepository) GetCTA(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingCTA, error) {
	if !scope.IsValid() {
		return domain.LandingCTA{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, name, label, type, target, destination, tracking_key, created_by, updated_by, created_at, updated_at
		FROM landing_ctas
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`

	var c domain.LandingCTA
	var createdBy, updatedBy *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&c.ID, &c.Name, &c.Label, &c.Type, &c.Target, &c.Destination, &c.TrackingKey,
			&createdBy, &updatedBy, &c.CreatedAt, &c.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingCTA{}, err
	}
	c.OrganizationID = scope.OrganizationID()
	if createdBy != nil {
		c.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		c.UpdatedBy = *updatedBy
	}

	return c, nil
}

func (r *reusableRepository) ListCTAs(ctx context.Context, scope coretenant.Scope) ([]domain.LandingCTA, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, name, label, type, target, destination, tracking_key, created_by, updated_by, created_at, updated_at
		FROM landing_ctas
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	var ctas []domain.LandingCTA

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c domain.LandingCTA
			var createdBy, updatedBy *string

			err := rows.Scan(
				&c.ID, &c.Name, &c.Label, &c.Type, &c.Target, &c.Destination, &c.TrackingKey,
				&createdBy, &updatedBy, &c.CreatedAt, &c.UpdatedAt,
			)
			if err != nil {
				return err
			}
			c.OrganizationID = scope.OrganizationID()
			if createdBy != nil {
				c.CreatedBy = *createdBy
			}
			if updatedBy != nil {
				c.UpdatedBy = *updatedBy
			}
			ctas = append(ctas, c)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return ctas, nil
}

func (r *reusableRepository) UpdateCTA(ctx context.Context, scope coretenant.Scope, id string, params UpdateCTAParams) (domain.LandingCTA, error) {
	if !scope.IsValid() {
		return domain.LandingCTA{}, coretenant.ErrInvalidScope
	}

	query := `UPDATE landing_ctas SET updated_at = NOW()`
	var args []interface{}
	argCount := 1

	if params.Name != nil {
		args = append(args, *params.Name)
		query += fmt.Sprintf(", name = $%d", argCount)
		argCount++
	}
	if params.Label != nil {
		args = append(args, *params.Label)
		query += fmt.Sprintf(", label = $%d", argCount)
		argCount++
	}
	if params.Type != nil {
		args = append(args, *params.Type)
		query += fmt.Sprintf(", type = $%d", argCount)
		argCount++
	}
	if params.Target != nil {
		args = append(args, *params.Target)
		query += fmt.Sprintf(", target = $%d", argCount)
		argCount++
	}
	if params.Destination != nil {
		args = append(args, *params.Destination)
		query += fmt.Sprintf(", destination = $%d", argCount)
		argCount++
	}
	if params.TrackingKey != nil {
		args = append(args, *params.TrackingKey)
		query += fmt.Sprintf(", tracking_key = $%d", argCount)
		argCount++
	}
	if params.UpdatedBy != "" {
		args = append(args, params.UpdatedBy)
		query += fmt.Sprintf(", updated_by = $%d", argCount)
		argCount++
	}

	args = append(args, id, scope.OrganizationID())
	query += fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", argCount, argCount+1)
	query += `id, name, label, type, target, destination, tracking_key, created_by, updated_by, created_at, updated_at`

	var c domain.LandingCTA
	var createdBy, updatedBy *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(
			&c.ID, &c.Name, &c.Label, &c.Type, &c.Target, &c.Destination, &c.TrackingKey,
			&createdBy, &updatedBy, &c.CreatedAt, &c.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingCTA{}, err
	}
	c.OrganizationID = scope.OrganizationID()
	if createdBy != nil {
		c.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		c.UpdatedBy = *updatedBy
	}

	return c, nil
}

func (r *reusableRepository) DeleteCTA(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE landing_ctas
		SET deleted_at = NOW(), updated_by = $1
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var ub interface{} = nil
		if deletedBy != "" {
			ub = deletedBy
		}
		cmdTag, err := tx.Exec(ctx, query, ub, id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *reusableRepository) CreatePricingPlan(ctx context.Context, scope coretenant.Scope, params CreatePricingPlanParams) (domain.LandingPricingPlan, error) {
	if !scope.IsValid() {
		return domain.LandingPricingPlan{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_pricing_plans (
			organization_id, name, price_label, interval_label, description, features,
			cta_label, cta_url, is_featured, is_enabled,
			sort_order, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			COALESCE((SELECT MAX(sort_order) + 1 FROM landing_pricing_plans WHERE organization_id = $1 AND deleted_at IS NULL), 0),
			$11
		) RETURNING
			id, name, price_label, interval_label, description, features, cta_label, cta_url,
			is_featured, is_enabled, sort_order, created_by, updated_by, created_at, updated_at
	`

	features := params.Features
	if features == nil {
		features = []string{}
	}

	var p domain.LandingPricingPlan
	var createdBy, updatedBy, intervalLabel, description *string

	if params.IntervalLabel != "" {
		intervalLabel = &params.IntervalLabel
	}
	if params.Description != "" {
		description = &params.Description
	}

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var cb interface{} = nil
		if params.CreatedBy != "" {
			cb = params.CreatedBy
		}

		return tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.Name,
			params.PriceLabel,
			intervalLabel,
			description,
			features,
			params.CTALabel,
			params.CTAURL,
			params.IsFeatured,
			params.IsEnabled,
			cb,
		).Scan(
			&p.ID, &p.Name, &p.PriceLabel, &intervalLabel, &description, &p.Features,
			&p.CTALabel, &p.CTAURL, &p.IsFeatured, &p.IsEnabled, &p.SortOrder,
			&createdBy, &updatedBy, &p.CreatedAt, &p.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingPricingPlan{}, err
	}
	p.OrganizationID = scope.OrganizationID()
	if intervalLabel != nil {
		p.IntervalLabel = *intervalLabel
	}
	if description != nil {
		p.Description = *description
	}
	if createdBy != nil {
		p.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		p.UpdatedBy = *updatedBy
	}

	return p, nil
}

func (r *reusableRepository) GetPricingPlan(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingPricingPlan, error) {
	if !scope.IsValid() {
		return domain.LandingPricingPlan{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, name, price_label, interval_label, description, features, cta_label, cta_url,
			is_featured, is_enabled, sort_order, created_by, updated_by, created_at, updated_at
		FROM landing_pricing_plans
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`

	var p domain.LandingPricingPlan
	var createdBy, updatedBy, intervalLabel, description *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&p.ID, &p.Name, &p.PriceLabel, &intervalLabel, &description, &p.Features,
			&p.CTALabel, &p.CTAURL, &p.IsFeatured, &p.IsEnabled, &p.SortOrder,
			&createdBy, &updatedBy, &p.CreatedAt, &p.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingPricingPlan{}, err
	}
	p.OrganizationID = scope.OrganizationID()
	if intervalLabel != nil {
		p.IntervalLabel = *intervalLabel
	}
	if description != nil {
		p.Description = *description
	}
	if createdBy != nil {
		p.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		p.UpdatedBy = *updatedBy
	}

	return p, nil
}

func (r *reusableRepository) ListPricingPlans(ctx context.Context, scope coretenant.Scope) ([]domain.LandingPricingPlan, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, name, price_label, interval_label, description, features, cta_label, cta_url,
			is_featured, is_enabled, sort_order, created_by, updated_by, created_at, updated_at
		FROM landing_pricing_plans
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY sort_order ASC, created_at ASC
	`

	var plans []domain.LandingPricingPlan

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var p domain.LandingPricingPlan
			var createdBy, updatedBy, intervalLabel, description *string

			err := rows.Scan(
				&p.ID, &p.Name, &p.PriceLabel, &intervalLabel, &description, &p.Features,
				&p.CTALabel, &p.CTAURL, &p.IsFeatured, &p.IsEnabled, &p.SortOrder,
				&createdBy, &updatedBy, &p.CreatedAt, &p.UpdatedAt,
			)
			if err != nil {
				return err
			}
			p.OrganizationID = scope.OrganizationID()
			if intervalLabel != nil {
				p.IntervalLabel = *intervalLabel
			}
			if description != nil {
				p.Description = *description
			}
			if createdBy != nil {
				p.CreatedBy = *createdBy
			}
			if updatedBy != nil {
				p.UpdatedBy = *updatedBy
			}
			plans = append(plans, p)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return plans, nil
}

func (r *reusableRepository) UpdatePricingPlan(ctx context.Context, scope coretenant.Scope, id string, params UpdatePricingPlanParams) (domain.LandingPricingPlan, error) {
	if !scope.IsValid() {
		return domain.LandingPricingPlan{}, coretenant.ErrInvalidScope
	}

	query := `UPDATE landing_pricing_plans SET updated_at = NOW()`
	var args []interface{}
	argCount := 1

	if params.Name != nil {
		args = append(args, *params.Name)
		query += fmt.Sprintf(", name = $%d", argCount)
		argCount++
	}
	if params.PriceLabel != nil {
		args = append(args, *params.PriceLabel)
		query += fmt.Sprintf(", price_label = $%d", argCount)
		argCount++
	}
	if params.IntervalLabel != nil {
		args = append(args, *params.IntervalLabel)
		query += fmt.Sprintf(", interval_label = $%d", argCount)
		argCount++
	}
	if params.Description != nil {
		args = append(args, *params.Description)
		query += fmt.Sprintf(", description = $%d", argCount)
		argCount++
	}
	if params.Features != nil {
		args = append(args, *params.Features)
		query += fmt.Sprintf(", features = $%d", argCount)
		argCount++
	}
	if params.CTALabel != nil {
		args = append(args, *params.CTALabel)
		query += fmt.Sprintf(", cta_label = $%d", argCount)
		argCount++
	}
	if params.CTAURL != nil {
		args = append(args, *params.CTAURL)
		query += fmt.Sprintf(", cta_url = $%d", argCount)
		argCount++
	}
	if params.IsFeatured != nil {
		args = append(args, *params.IsFeatured)
		query += fmt.Sprintf(", is_featured = $%d", argCount)
		argCount++
	}
	if params.IsEnabled != nil {
		args = append(args, *params.IsEnabled)
		query += fmt.Sprintf(", is_enabled = $%d", argCount)
		argCount++
	}
	if params.UpdatedBy != "" {
		args = append(args, params.UpdatedBy)
		query += fmt.Sprintf(", updated_by = $%d", argCount)
		argCount++
	}

	args = append(args, id, scope.OrganizationID())
	query += fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d AND deleted_at IS NULL RETURNING ", argCount, argCount+1)
	query += `id, name, price_label, interval_label, description, features, cta_label, cta_url,
		is_featured, is_enabled, sort_order, created_by, updated_by, created_at, updated_at`

	var p domain.LandingPricingPlan
	var createdBy, updatedBy, intervalLabel, description *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(
			&p.ID, &p.Name, &p.PriceLabel, &intervalLabel, &description, &p.Features,
			&p.CTALabel, &p.CTAURL, &p.IsFeatured, &p.IsEnabled, &p.SortOrder,
			&createdBy, &updatedBy, &p.CreatedAt, &p.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingPricingPlan{}, err
	}
	p.OrganizationID = scope.OrganizationID()
	if intervalLabel != nil {
		p.IntervalLabel = *intervalLabel
	}
	if description != nil {
		p.Description = *description
	}
	if createdBy != nil {
		p.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		p.UpdatedBy = *updatedBy
	}

	return p, nil
}

func (r *reusableRepository) ReorderPricingPlans(ctx context.Context, scope coretenant.Scope, planIDs []string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		query1 := `
			UPDATE landing_pricing_plans
			SET sort_order = sort_order + 1000000, updated_at = NOW()
			WHERE organization_id = $1
		`
		_, err := tx.Exec(ctx, query1, scope.OrganizationID())
		if err != nil {
			return err
		}

		query2 := `
			UPDATE landing_pricing_plans
			SET sort_order = $1, updated_at = NOW()
			WHERE id = $2 AND organization_id = $3
		`
		for i, id := range planIDs {
			cmdTag, err := tx.Exec(ctx, query2, i, id, scope.OrganizationID())
			if err != nil {
				return err
			}
			if cmdTag.RowsAffected() == 0 {
				return pgx.ErrNoRows
			}
		}

		return nil
	})
}

func (r *reusableRepository) DeletePricingPlan(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE landing_pricing_plans
		SET deleted_at = NOW(), updated_by = $1
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var ub interface{} = nil
		if deletedBy != "" {
			ub = deletedBy
		}
		cmdTag, err := tx.Exec(ctx, query, ub, id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *reusableRepository) CreateMenu(ctx context.Context, scope coretenant.Scope, params CreateMenuParams) (domain.LandingMenu, error) {
	if !scope.IsValid() {
		return domain.LandingMenu{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_menus (
			organization_id, name, location, is_active, created_by
		) VALUES (
			$1, $2, $3, $4, $5
		) RETURNING
			id, name, location, is_active, created_by, updated_by, created_at, updated_at
	`

	var m domain.LandingMenu
	var createdBy, updatedBy *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var cb interface{} = nil
		if params.CreatedBy != "" {
			cb = params.CreatedBy
		}

		err := tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.Name,
			params.Location,
			params.IsActive,
			cb,
		).Scan(
			&m.ID, &m.Name, &m.Location, &m.IsActive,
			&createdBy, &updatedBy, &m.CreatedAt, &m.UpdatedAt,
		)
		return err
	})

	if err != nil {
		return domain.LandingMenu{}, err
	}
	m.OrganizationID = scope.OrganizationID()
	if createdBy != nil {
		m.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		m.UpdatedBy = *updatedBy
	}

	return m, nil
}

func (r *reusableRepository) GetMenu(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingMenu, error) {
	if !scope.IsValid() {
		return domain.LandingMenu{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, name, location, is_active, created_by, updated_by, created_at, updated_at
		FROM landing_menus
		WHERE id = $1 AND organization_id = $2 AND deleted_at IS NULL
	`

	var m domain.LandingMenu
	var createdBy, updatedBy *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&m.ID, &m.Name, &m.Location, &m.IsActive,
			&createdBy, &updatedBy, &m.CreatedAt, &m.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingMenu{}, err
	}
	m.OrganizationID = scope.OrganizationID()
	if createdBy != nil {
		m.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		m.UpdatedBy = *updatedBy
	}

	return m, nil
}

func (r *reusableRepository) ListMenus(ctx context.Context, scope coretenant.Scope) ([]domain.LandingMenu, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, name, location, is_active, created_by, updated_by, created_at, updated_at
		FROM landing_menus
		WHERE organization_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`

	var menus []domain.LandingMenu

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m domain.LandingMenu
			var createdBy, updatedBy *string

			err := rows.Scan(
				&m.ID, &m.Name, &m.Location, &m.IsActive,
				&createdBy, &updatedBy, &m.CreatedAt, &m.UpdatedAt,
			)
			if err != nil {
				return err
			}
			m.OrganizationID = scope.OrganizationID()
			if createdBy != nil {
				m.CreatedBy = *createdBy
			}
			if updatedBy != nil {
				m.UpdatedBy = *updatedBy
			}
			menus = append(menus, m)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return menus, nil
}

func (r *reusableRepository) UpdateMenu(ctx context.Context, scope coretenant.Scope, id string, params UpdateMenuParams) (domain.LandingMenu, error) {
	if !scope.IsValid() {
		return domain.LandingMenu{}, coretenant.ErrInvalidScope
	}

	query := `UPDATE landing_menus SET updated_at = NOW()`
	var args []interface{}
	argCount := 1

	if params.Name != nil {
		args = append(args, *params.Name)
		query += fmt.Sprintf(", name = $%d", argCount)
		argCount++
	}
	if params.Location != nil {
		args = append(args, *params.Location)
		query += fmt.Sprintf(", location = $%d", argCount)
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
	query += `id, name, location, is_active, created_by, updated_by, created_at, updated_at`

	var m domain.LandingMenu
	var createdBy, updatedBy *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(
			&m.ID, &m.Name, &m.Location, &m.IsActive,
			&createdBy, &updatedBy, &m.CreatedAt, &m.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingMenu{}, err
	}
	m.OrganizationID = scope.OrganizationID()
	if createdBy != nil {
		m.CreatedBy = *createdBy
	}
	if updatedBy != nil {
		m.UpdatedBy = *updatedBy
	}

	return m, nil
}

func (r *reusableRepository) DeleteMenu(ctx context.Context, scope coretenant.Scope, id string, deletedBy string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		UPDATE landing_menus
		SET deleted_at = NOW(), updated_by = $1
		WHERE id = $2 AND organization_id = $3 AND deleted_at IS NULL
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		var ub interface{} = nil
		if deletedBy != "" {
			ub = deletedBy
		}
		cmdTag, err := tx.Exec(ctx, query, ub, id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *reusableRepository) CreateMenuItem(ctx context.Context, scope coretenant.Scope, params CreateMenuItemParams) (domain.LandingMenuItem, error) {
	if !scope.IsValid() {
		return domain.LandingMenuItem{}, coretenant.ErrInvalidScope
	}

	// For atomic high offset reordering logic, same as sections/fields
	query := `
		INSERT INTO landing_menu_items (
			organization_id, menu_id, parent_id, label, link_type, destination, target, sort_order, is_enabled
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		) RETURNING
			id, menu_id, parent_id, label, link_type, destination, target, sort_order, is_enabled, created_at, updated_at
	`

	var m domain.LandingMenuItem
	var parentID *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.MenuID,
			params.ParentID,
			params.Label,
			params.LinkType,
			params.Destination,
			params.Target,
			params.SortOrder,
			params.IsEnabled,
		).Scan(
			&m.ID, &m.MenuID, &parentID, &m.Label, &m.LinkType, &m.Destination, &m.Target,
			&m.SortOrder, &m.IsEnabled, &m.CreatedAt, &m.UpdatedAt,
		)
		return err
	})

	if err != nil {
		return domain.LandingMenuItem{}, err
	}
	m.OrganizationID = scope.OrganizationID()
	if parentID != nil {
		m.ParentID = parentID
	}

	return m, nil
}

func (r *reusableRepository) GetMenuItem(ctx context.Context, scope coretenant.Scope, id string) (domain.LandingMenuItem, error) {
	if !scope.IsValid() {
		return domain.LandingMenuItem{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, menu_id, parent_id, label, link_type, destination, target, sort_order, is_enabled, created_at, updated_at
		FROM landing_menu_items
		WHERE id = $1 AND organization_id = $2
	`

	var m domain.LandingMenuItem
	var parentID *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, id, scope.OrganizationID()).Scan(
			&m.ID, &m.MenuID, &parentID, &m.Label, &m.LinkType, &m.Destination, &m.Target,
			&m.SortOrder, &m.IsEnabled, &m.CreatedAt, &m.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingMenuItem{}, err
	}
	m.OrganizationID = scope.OrganizationID()
	if parentID != nil {
		m.ParentID = parentID
	}

	return m, nil
}

func (r *reusableRepository) UpdateMenuItem(ctx context.Context, scope coretenant.Scope, id string, params UpdateMenuItemParams) (domain.LandingMenuItem, error) {
	if !scope.IsValid() {
		return domain.LandingMenuItem{}, coretenant.ErrInvalidScope
	}

	query := `UPDATE landing_menu_items SET updated_at = NOW()`
	var args []interface{}
	argCount := 1

	if params.ParentID != nil {
		if *params.ParentID == "" {
			query += ", parent_id = NULL"
		} else {
			args = append(args, *params.ParentID)
			query += fmt.Sprintf(", parent_id = $%d", argCount)
			argCount++
		}
	}
	if params.Label != nil {
		args = append(args, *params.Label)
		query += fmt.Sprintf(", label = $%d", argCount)
		argCount++
	}
	if params.LinkType != nil {
		args = append(args, *params.LinkType)
		query += fmt.Sprintf(", link_type = $%d", argCount)
		argCount++
	}
	if params.Destination != nil {
		args = append(args, *params.Destination)
		query += fmt.Sprintf(", destination = $%d", argCount)
		argCount++
	}
	if params.Target != nil {
		args = append(args, *params.Target)
		query += fmt.Sprintf(", target = $%d", argCount)
		argCount++
	}
	if params.SortOrder != nil {
		args = append(args, *params.SortOrder)
		query += fmt.Sprintf(", sort_order = $%d", argCount)
		argCount++
	}
	if params.IsEnabled != nil {
		args = append(args, *params.IsEnabled)
		query += fmt.Sprintf(", is_enabled = $%d", argCount)
		argCount++
	}

	args = append(args, id, scope.OrganizationID())
	query += fmt.Sprintf(" WHERE id = $%d AND organization_id = $%d RETURNING ", argCount, argCount+1)
	query += `id, menu_id, parent_id, label, link_type, destination, target, sort_order, is_enabled, created_at, updated_at`

	var m domain.LandingMenuItem
	var parentID *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(
			&m.ID, &m.MenuID, &parentID, &m.Label, &m.LinkType, &m.Destination, &m.Target,
			&m.SortOrder, &m.IsEnabled, &m.CreatedAt, &m.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingMenuItem{}, err
	}
	m.OrganizationID = scope.OrganizationID()
	if parentID != nil {
		m.ParentID = parentID
	}

	return m, nil
}

func (r *reusableRepository) ListMenuItems(ctx context.Context, scope coretenant.Scope, menuID string) ([]domain.LandingMenuItem, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, menu_id, parent_id, label, link_type, destination, target, sort_order, is_enabled, created_at, updated_at
		FROM landing_menu_items
		WHERE organization_id = $1 AND menu_id = $2
		ORDER BY sort_order ASC
	`

	var items []domain.LandingMenuItem

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID(), menuID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var m domain.LandingMenuItem
			var parentID *string

			err := rows.Scan(
				&m.ID, &m.MenuID, &parentID, &m.Label, &m.LinkType, &m.Destination, &m.Target,
				&m.SortOrder, &m.IsEnabled, &m.CreatedAt, &m.UpdatedAt,
			)
			if err != nil {
				return err
			}
			m.OrganizationID = scope.OrganizationID()
			if parentID != nil {
				m.ParentID = parentID
			}
			items = append(items, m)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return items, nil
}

func (r *reusableRepository) ReorderMenuItems(ctx context.Context, scope coretenant.Scope, menuID string, itemIDs []string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		query1 := `
			UPDATE landing_menu_items
			SET sort_order = sort_order + 1000000, updated_at = NOW()
			WHERE organization_id = $1 AND menu_id = $2
		`
		_, err := tx.Exec(ctx, query1, scope.OrganizationID(), menuID)
		if err != nil {
			return err
		}

		query2 := `
			UPDATE landing_menu_items
			SET sort_order = $1, updated_at = NOW()
			WHERE id = $2 AND organization_id = $3 AND menu_id = $4
		`
		for i, id := range itemIDs {
			cmdTag, err := tx.Exec(ctx, query2, i, id, scope.OrganizationID(), menuID)
			if err != nil {
				return err
			}
			if cmdTag.RowsAffected() == 0 {
				return pgx.ErrNoRows
			}
		}

		return nil
	})
}

func (r *reusableRepository) DeleteMenuItem(ctx context.Context, scope coretenant.Scope, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		DELETE FROM landing_menu_items
		WHERE id = $1 AND organization_id = $2
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, id, scope.OrganizationID())
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}
