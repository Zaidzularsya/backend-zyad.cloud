package repository

import (
	"context"

	"github.com/jackc/pgx/v5"

	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

type brandingRepository struct {
	db *database.Pool
}

func NewBrandingRepository(db *database.Pool) BrandingRepository {
	return &brandingRepository{db: db}
}

func (r *brandingRepository) withTx(ctx context.Context, scope coretenant.Scope, fn func(pgx.Tx) error) error {
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

func (r *brandingRepository) Upsert(ctx context.Context, scope coretenant.Scope, params CreateBrandingParams) (domain.LandingBranding, error) {
	if !scope.IsValid() {
		return domain.LandingBranding{}, coretenant.ErrInvalidScope
	}

	colors := params.Colors
	if colors == nil {
		colors = &domain.BrandingColors{}
	}
	typography := params.Typography
	if typography == nil {
		typography = &domain.BrandingTypography{}
	}
	shape := params.Shape
	if shape == nil {
		shape = &domain.BrandingShape{}
	}
	layout := params.Layout
	if layout == nil {
		layout = &domain.BrandingLayout{}
	}
	contact := params.Contact
	if contact == nil {
		contact = &domain.BrandingContact{}
	}
	socialLinks := params.SocialLinks
	if socialLinks == nil {
		socialLinks = []domain.BrandingSocialLink{}
	}

	var landingPageID interface{} = nil
	if params.LandingPageID != nil {
		landingPageID = *params.LandingPageID
	}

	// We use ON CONFLICT depending on if landing_page_id is NULL or not.
	// If it's NULL, we upsert the default org branding.
	// If it's NOT NULL, we upsert the page specific override.
	var query string
	if params.LandingPageID == nil {
		query = `
			INSERT INTO landing_brandings (
				organization_id, landing_page_id, company_name, tagline,
				logo_light_url, logo_dark_url, favicon_url, social_image_url,
				colors, typography, shape, layout, contact, social_links
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
			)
			ON CONFLICT (organization_id) WHERE landing_page_id IS NULL DO UPDATE SET
				company_name = EXCLUDED.company_name,
				tagline = EXCLUDED.tagline,
				logo_light_url = EXCLUDED.logo_light_url,
				logo_dark_url = EXCLUDED.logo_dark_url,
				favicon_url = EXCLUDED.favicon_url,
				social_image_url = EXCLUDED.social_image_url,
				colors = EXCLUDED.colors,
				typography = EXCLUDED.typography,
				shape = EXCLUDED.shape,
				layout = EXCLUDED.layout,
				contact = EXCLUDED.contact,
				social_links = EXCLUDED.social_links,
				updated_at = NOW()
			RETURNING
				id, landing_page_id, company_name, tagline,
				logo_light_url, logo_dark_url, favicon_url, social_image_url,
				colors, typography, shape, layout, contact, social_links,
				created_at, updated_at
		`
	} else {
		query = `
			INSERT INTO landing_brandings (
				organization_id, landing_page_id, company_name, tagline,
				logo_light_url, logo_dark_url, favicon_url, social_image_url,
				colors, typography, shape, layout, contact, social_links
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
			)
			ON CONFLICT (organization_id, landing_page_id) WHERE landing_page_id IS NOT NULL DO UPDATE SET
				company_name = EXCLUDED.company_name,
				tagline = EXCLUDED.tagline,
				logo_light_url = EXCLUDED.logo_light_url,
				logo_dark_url = EXCLUDED.logo_dark_url,
				favicon_url = EXCLUDED.favicon_url,
				social_image_url = EXCLUDED.social_image_url,
				colors = EXCLUDED.colors,
				typography = EXCLUDED.typography,
				shape = EXCLUDED.shape,
				layout = EXCLUDED.layout,
				contact = EXCLUDED.contact,
				social_links = EXCLUDED.social_links,
				updated_at = NOW()
			RETURNING
				id, landing_page_id, company_name, tagline,
				logo_light_url, logo_dark_url, favicon_url, social_image_url,
				colors, typography, shape, layout, contact, social_links,
				created_at, updated_at
		`
	}

	var branding domain.LandingBranding
	var pageID, companyName, tagline, logoLight, logoDark, favicon, socialImage *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			landingPageID,
			params.CompanyName,
			params.Tagline,
			params.LogoLightURL,
			params.LogoDarkURL,
			params.FaviconURL,
			params.SocialImageURL,
			colors,
			typography,
			shape,
			layout,
			contact,
			socialLinks,
		).Scan(
			&branding.ID, &pageID, &companyName, &tagline,
			&logoLight, &logoDark, &favicon, &socialImage,
			&branding.Colors, &branding.Typography, &branding.Shape, &branding.Layout, &branding.Contact, &branding.SocialLinks,
			&branding.CreatedAt, &branding.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingBranding{}, err
	}

	branding.OrganizationID = scope.OrganizationID()
	branding.LandingPageID = pageID
	if companyName != nil { branding.CompanyName = *companyName }
	if tagline != nil { branding.Tagline = *tagline }
	if logoLight != nil { branding.LogoLightURL = *logoLight }
	if logoDark != nil { branding.LogoDarkURL = *logoDark }
	if favicon != nil { branding.FaviconURL = *favicon }
	if socialImage != nil { branding.SocialImageURL = *socialImage }

	return branding, nil
}

func (r *brandingRepository) GetDefault(ctx context.Context, scope coretenant.Scope) (domain.LandingBranding, error) {
	if !scope.IsValid() {
		return domain.LandingBranding{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, company_name, tagline,
			logo_light_url, logo_dark_url, favicon_url, social_image_url,
			colors, typography, shape, layout, contact, social_links,
			created_at, updated_at
		FROM landing_brandings
		WHERE organization_id = $1 AND landing_page_id IS NULL
	`

	var branding domain.LandingBranding
	var pageID, companyName, tagline, logoLight, logoDark, favicon, socialImage *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, scope.OrganizationID()).Scan(
			&branding.ID, &pageID, &companyName, &tagline,
			&logoLight, &logoDark, &favicon, &socialImage,
			&branding.Colors, &branding.Typography, &branding.Shape, &branding.Layout, &branding.Contact, &branding.SocialLinks,
			&branding.CreatedAt, &branding.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingBranding{}, err
	}

	branding.OrganizationID = scope.OrganizationID()
	branding.LandingPageID = pageID
	if companyName != nil { branding.CompanyName = *companyName }
	if tagline != nil { branding.Tagline = *tagline }
	if logoLight != nil { branding.LogoLightURL = *logoLight }
	if logoDark != nil { branding.LogoDarkURL = *logoDark }
	if favicon != nil { branding.FaviconURL = *favicon }
	if socialImage != nil { branding.SocialImageURL = *socialImage }

	return branding, nil
}

func (r *brandingRepository) GetByPage(ctx context.Context, scope coretenant.Scope, pageID string) (domain.LandingBranding, error) {
	if !scope.IsValid() {
		return domain.LandingBranding{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, landing_page_id, company_name, tagline,
			logo_light_url, logo_dark_url, favicon_url, social_image_url,
			colors, typography, shape, layout, contact, social_links,
			created_at, updated_at
		FROM landing_brandings
		WHERE organization_id = $1 AND landing_page_id = $2
	`

	var branding domain.LandingBranding
	var retPageID, companyName, tagline, logoLight, logoDark, favicon, socialImage *string

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, scope.OrganizationID(), pageID).Scan(
			&branding.ID, &retPageID, &companyName, &tagline,
			&logoLight, &logoDark, &favicon, &socialImage,
			&branding.Colors, &branding.Typography, &branding.Shape, &branding.Layout, &branding.Contact, &branding.SocialLinks,
			&branding.CreatedAt, &branding.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingBranding{}, err
	}

	branding.OrganizationID = scope.OrganizationID()
	branding.LandingPageID = retPageID
	if companyName != nil { branding.CompanyName = *companyName }
	if tagline != nil { branding.Tagline = *tagline }
	if logoLight != nil { branding.LogoLightURL = *logoLight }
	if logoDark != nil { branding.LogoDarkURL = *logoDark }
	if favicon != nil { branding.FaviconURL = *favicon }
	if socialImage != nil { branding.SocialImageURL = *socialImage }

	return branding, nil
}

func (r *brandingRepository) DeleteByPage(ctx context.Context, scope coretenant.Scope, pageID string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		DELETE FROM landing_brandings
		WHERE organization_id = $1 AND landing_page_id = $2
	`

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		cmdTag, err := tx.Exec(ctx, query, scope.OrganizationID(), pageID)
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return nil
	})
}

func (r *brandingRepository) CreateBinding(ctx context.Context, scope coretenant.Scope, params CreateDomainBindingParams) (domain.LandingDomainBinding, error) {
	if !scope.IsValid() {
		return domain.LandingDomainBinding{}, coretenant.ErrInvalidScope
	}

	query := `
		INSERT INTO landing_domain_bindings (
			organization_id, organization_domain_id, landing_page_id, is_primary
		) VALUES (
			$1, $2, $3, $4
		) RETURNING
			id, organization_domain_id, landing_page_id, is_primary, created_at, updated_at
	`

	var binding domain.LandingDomainBinding

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query,
			scope.OrganizationID(),
			params.OrganizationDomainID,
			params.LandingPageID,
			params.IsPrimary,
		).Scan(
			&binding.ID, &binding.OrganizationDomainID, &binding.LandingPageID,
			&binding.IsPrimary, &binding.CreatedAt, &binding.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingDomainBinding{}, err
	}
	binding.OrganizationID = scope.OrganizationID()

	return binding, nil
}

func (r *brandingRepository) ListBindings(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingDomainBinding, error) {
	if !scope.IsValid() {
		return nil, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, organization_domain_id, landing_page_id, is_primary, created_at, updated_at
		FROM landing_domain_bindings
		WHERE organization_id = $1 AND landing_page_id = $2
		ORDER BY created_at ASC
	`

	var bindings []domain.LandingDomainBinding

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID(), pageID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var b domain.LandingDomainBinding
			err := rows.Scan(
				&b.ID, &b.OrganizationDomainID, &b.LandingPageID,
				&b.IsPrimary, &b.CreatedAt, &b.UpdatedAt,
			)
			if err != nil {
				return err
			}
			b.OrganizationID = scope.OrganizationID()
			bindings = append(bindings, b)
		}
		return rows.Err()
	})

	if err != nil {
		return nil, err
	}

	return bindings, nil
}

func (r *brandingRepository) GetBindingByDomain(ctx context.Context, scope coretenant.Scope, domainID string) (domain.LandingDomainBinding, error) {
	if !scope.IsValid() {
		return domain.LandingDomainBinding{}, coretenant.ErrInvalidScope
	}

	query := `
		SELECT
			id, organization_domain_id, landing_page_id, is_primary, created_at, updated_at
		FROM landing_domain_bindings
		WHERE organization_id = $1 AND organization_domain_id = $2
	`

	var binding domain.LandingDomainBinding

	err := r.withTx(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, scope.OrganizationID(), domainID).Scan(
			&binding.ID, &binding.OrganizationDomainID, &binding.LandingPageID,
			&binding.IsPrimary, &binding.CreatedAt, &binding.UpdatedAt,
		)
	})

	if err != nil {
		return domain.LandingDomainBinding{}, err
	}
	binding.OrganizationID = scope.OrganizationID()

	return binding, nil
}

func (r *brandingRepository) DeleteBinding(ctx context.Context, scope coretenant.Scope, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	query := `
		DELETE FROM landing_domain_bindings
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

func (r *brandingRepository) SetPrimaryBinding(ctx context.Context, scope coretenant.Scope, pageID string, id string) error {
	if !scope.IsValid() {
		return coretenant.ErrInvalidScope
	}

	return r.withTx(ctx, scope, func(tx pgx.Tx) error {
		query1 := `
			UPDATE landing_domain_bindings
			SET is_primary = false, updated_at = NOW()
			WHERE organization_id = $1 AND landing_page_id = $2 AND is_primary = true
		`
		_, err := tx.Exec(ctx, query1, scope.OrganizationID(), pageID)
		if err != nil {
			return err
		}

		query2 := `
			UPDATE landing_domain_bindings
			SET is_primary = true, updated_at = NOW()
			WHERE id = $1 AND organization_id = $2 AND landing_page_id = $3
		`
		cmdTag, err := tx.Exec(ctx, query2, id, scope.OrganizationID(), pageID)
		if err != nil {
			return err
		}
		if cmdTag.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}

		return nil
	})
}
