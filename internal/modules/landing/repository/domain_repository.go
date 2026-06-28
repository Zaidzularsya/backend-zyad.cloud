package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

type defaultDomainRepository struct {
	db *database.Pool
}

func NewDomainRepository(db *database.Pool) DomainRepository {
	return &defaultDomainRepository{
		db: db,
	}
}

func (r *defaultDomainRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

func (r *defaultDomainRepository) BindDomain(ctx context.Context, scope coretenant.Scope, params BindDomainParams) (domain.DomainBinding, error) {
	if !scope.IsValid() {
		return domain.DomainBinding{}, coretenant.ErrInvalidScope
	}

	var binding domain.DomainBinding
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO landing_domain_bindings (organization_id, organization_domain_id, landing_page_id, is_primary)
			VALUES ($1, $2, $3, $4)
			RETURNING id, organization_id, organization_domain_id, landing_page_id, is_primary, created_at, updated_at
		`, scope.OrganizationID(), params.OrganizationDomainID, params.LandingPageID, params.IsPrimary).
			Scan(
				&binding.ID,
				&binding.OrganizationID,
				&binding.OrganizationDomainID,
				&binding.LandingPageID,
				&binding.IsPrimary,
				&binding.CreatedAt,
				&binding.UpdatedAt,
			)
	})

	if err != nil {
		return domain.DomainBinding{}, fmt.Errorf("bind domain: %w", err)
	}
	return binding, nil
}

func (r *defaultDomainRepository) UnbindDomain(ctx context.Context, scope coretenant.Scope, bindingID string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			DELETE FROM landing_domain_bindings
			WHERE id = $1
		`, bindingID)
		if err != nil {
			return fmt.Errorf("unbind domain: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return errors.New("domain binding not found")
		}
		return nil
	})
}

func (r *defaultDomainRepository) SetPrimaryBinding(ctx context.Context, scope coretenant.Scope, pageID string, bindingID string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		// Unset primary for all bindings of this page
		_, err := tx.Exec(ctx, `
			UPDATE landing_domain_bindings
			SET is_primary = false, updated_at = now()
			WHERE landing_page_id = $1 AND is_primary = true
		`, pageID)
		if err != nil {
			return fmt.Errorf("unset primary bindings: %w", err)
		}

		// Set primary for the specific binding
		tag, err := tx.Exec(ctx, `
			UPDATE landing_domain_bindings
			SET is_primary = true, updated_at = now()
			WHERE id = $1 AND landing_page_id = $2
		`, bindingID, pageID)
		if err != nil {
			return fmt.Errorf("set primary binding: %w", err)
		}

		if tag.RowsAffected() == 0 {
			return errors.New("domain binding not found or not associated with page")
		}

		return nil
	})
}

func (r *defaultDomainRepository) ListBindings(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.DomainBinding, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	bindings := make([]domain.DomainBinding, 0)
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, organization_id, organization_domain_id, landing_page_id, is_primary, created_at, updated_at
			FROM landing_domain_bindings
			WHERE landing_page_id = $1
			ORDER BY created_at ASC
		`, pageID)
		if err != nil {
			return fmt.Errorf("list domain bindings: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var b domain.DomainBinding
			if err := rows.Scan(
				&b.ID,
				&b.OrganizationID,
				&b.OrganizationDomainID,
				&b.LandingPageID,
				&b.IsPrimary,
				&b.CreatedAt,
				&b.UpdatedAt,
			); err != nil {
				return fmt.Errorf("scan domain binding: %w", err)
			}
			bindings = append(bindings, b)
		}

		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate domain bindings: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return bindings, nil
}

func (r *defaultDomainRepository) ListAvailableDomains(ctx context.Context, scope coretenant.Scope) ([]domain.AvailableDomain, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	domains := make([]domain.AvailableDomain, 0)
	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		// Find verified or active domains for this organization
		rows, err := tx.Query(ctx, `
			SELECT id, organization_id, type, canonical_host, status, ssl_status
			FROM organization_domains
			WHERE organization_id = $1 
			  AND status IN ('verified', 'active') 
			  AND deleted_at IS NULL
			ORDER BY created_at ASC
		`, scope.OrganizationID())
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("list available domains: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var d domain.AvailableDomain
			if err := rows.Scan(
				&d.ID,
				&d.OrganizationID,
				&d.Type,
				&d.CanonicalHost,
				&d.Status,
				&d.SSLStatus,
			); err != nil {
				return fmt.Errorf("scan available domain: %w", err)
			}
			domains = append(domains, d)
		}

		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate available domains: %w", err)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return domains, nil
}
