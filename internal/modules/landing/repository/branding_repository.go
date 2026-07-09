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
	if companyName != nil {
		branding.CompanyName = *companyName
	}
	if tagline != nil {
		branding.Tagline = *tagline
	}
	if logoLight != nil {
		branding.LogoLightURL = *logoLight
	}
	if logoDark != nil {
		branding.LogoDarkURL = *logoDark
	}
	if favicon != nil {
		branding.FaviconURL = *favicon
	}
	if socialImage != nil {
		branding.SocialImageURL = *socialImage
	}

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
	if companyName != nil {
		branding.CompanyName = *companyName
	}
	if tagline != nil {
		branding.Tagline = *tagline
	}
	if logoLight != nil {
		branding.LogoLightURL = *logoLight
	}
	if logoDark != nil {
		branding.LogoDarkURL = *logoDark
	}
	if favicon != nil {
		branding.FaviconURL = *favicon
	}
	if socialImage != nil {
		branding.SocialImageURL = *socialImage
	}

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
	if companyName != nil {
		branding.CompanyName = *companyName
	}
	if tagline != nil {
		branding.Tagline = *tagline
	}
	if logoLight != nil {
		branding.LogoLightURL = *logoLight
	}
	if logoDark != nil {
		branding.LogoDarkURL = *logoDark
	}
	if favicon != nil {
		branding.FaviconURL = *favicon
	}
	if socialImage != nil {
		branding.SocialImageURL = *socialImage
	}

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
