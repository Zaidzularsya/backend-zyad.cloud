package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	coretenant "zyad.cloud/internal/core/tenant"
	"zyad.cloud/internal/modules/landing/domain"
	"zyad.cloud/internal/platform/database"
)

type resolverRepository struct {
	db *database.Pool
}

func NewResolverRepository(db *database.Pool) ResolverRepository {
	return &resolverRepository{
		db: db,
	}
}

func (r *resolverRepository) ResolveBySlug(ctx context.Context, scope coretenant.Scope, slug string) (domain.LandingPage, error) {
	query := `
		SELECT
			id, organization_id, name, title, slug, page_type, status,
			visibility, password_hash, seo, settings, locale, timezone,
			is_homepage, is_template, created_by, created_at, updated_at, builder
		FROM landing_pages
		WHERE organization_id = $1
			AND lower(slug) = lower($2)
			AND deleted_at IS NULL
			AND is_template = false
	`

	var page domain.LandingPage
	var passwordHash *string
	var createdBy *string
	err := r.within(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, scope.OrganizationID(), slug).Scan(
			&page.ID, &page.OrganizationID, &page.Name, &page.Title, &page.Slug,
			&page.Type, &page.Status, &page.Visibility, &passwordHash,
			&page.SEO, &page.Settings, &page.Locale, &page.Timezone, &page.IsHomepage, &page.IsTemplate,
			&createdBy, &page.CreatedAt, &page.UpdatedAt, &page.Builder,
		)
	})

	if passwordHash != nil {
		page.PasswordHash = *passwordHash
	}
	if createdBy != nil {
		page.CreatedBy = *createdBy
	}

	if err != nil {
		return domain.LandingPage{}, err
	}

	return page, nil
}

func (r *resolverRepository) ResolveByDomain(ctx context.Context, scope coretenant.Scope, customDomain string) (domain.LandingPage, error) {
	// Explicit page binding wins. When no binding exists, the tenant primary domain
	// falls back to the published homepage so first-domain setup works out of the box.
	query := `
		SELECT
			p.id, p.organization_id, p.name, p.title, p.slug, p.page_type, p.status,
			p.visibility, p.password_hash, p.seo, p.settings, p.locale, p.timezone,
			p.is_homepage, p.is_template, p.created_by, p.created_at, p.updated_at, p.builder
		FROM organization_domains d
		JOIN landing_pages p ON p.organization_id = d.organization_id
		LEFT JOIN landing_domain_bindings b
			ON b.organization_domain_id = d.id
			AND b.landing_page_id = p.id
		WHERE d.canonical_host = $1
			AND d.status = 'active'
			AND d.deleted_at IS NULL
			AND p.deleted_at IS NULL
			AND p.is_template = false
			AND (
				b.id IS NOT NULL
				OR (d.is_primary = true AND p.is_homepage = true)
			)
		ORDER BY
			CASE WHEN b.is_primary = true THEN 0 WHEN b.id IS NOT NULL THEN 1 ELSE 2 END,
			p.is_homepage DESC,
			p.created_at ASC
		LIMIT 1
	`

	var page domain.LandingPage
	var passwordHash *string
	var createdBy *string
	err := r.within(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, customDomain).Scan(
			&page.ID, &page.OrganizationID, &page.Name, &page.Title, &page.Slug,
			&page.Type, &page.Status, &page.Visibility, &passwordHash,
			&page.SEO, &page.Settings, &page.Locale, &page.Timezone, &page.IsHomepage, &page.IsTemplate,
			&createdBy, &page.CreatedAt, &page.UpdatedAt, &page.Builder,
		)
	})

	if passwordHash != nil {
		page.PasswordHash = *passwordHash
	}
	if createdBy != nil {
		page.CreatedBy = *createdBy
	}

	if err != nil {
		return domain.LandingPage{}, err
	}

	return page, nil
}

func (r *resolverRepository) ResolveSections(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingSection, error) {
	query := `
		SELECT
			id, landing_page_id, section_key, section_type, name,
			sort_order, is_enabled, content, style, created_at, updated_at
		FROM landing_page_sections
		WHERE landing_page_id = $1 AND deleted_at IS NULL
		ORDER BY sort_order ASC
	`

	var sections []domain.LandingSection

	err := r.within(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, pageID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var section domain.LandingSection
			err := rows.Scan(
				&section.ID, &section.LandingPageID, &section.Key, &section.Type, &section.Name,
				&section.SortOrder, &section.IsEnabled, &section.Content, &section.Style,
				&section.CreatedAt, &section.UpdatedAt,
			)
			if err != nil {
				return err
			}
			sections = append(sections, section)
		}

		return rows.Err()
	})

	return sections, err
}

func (r *resolverRepository) ResolveForms(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingForm, error) {
	query := `
		SELECT
			id, landing_page_id, name, key, description,
			submit_label, success_message, redirect_url, is_active,
			created_at, updated_at
		FROM landing_forms
		WHERE landing_page_id = $1 AND deleted_at IS NULL
	`

	var forms []domain.LandingForm

	err := r.within(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, pageID)
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

	return forms, err
}

func (r *resolverRepository) ResolveBranding(ctx context.Context, scope coretenant.Scope, pageID string) (domain.LandingBranding, error) {
	query := `
		SELECT
			id, organization_id, landing_page_id, company_name, tagline,
			logo_light_url, logo_dark_url, favicon_url, social_image_url,
			colors, typography, shape, layout, contact, social_links,
			created_at, updated_at
		FROM landing_brandings
		WHERE landing_page_id = $1 AND deleted_at IS NULL
	`

	var branding domain.LandingBranding
	var orgID *string
	var pageIDPtr *string

	err := r.within(ctx, scope, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, pageID).Scan(
			&branding.ID, &orgID, &pageIDPtr, &branding.CompanyName, &branding.Tagline,
			&branding.LogoLightURL, &branding.LogoDarkURL, &branding.FaviconURL, &branding.SocialImageURL,
			&branding.Colors, &branding.Typography, &branding.Shape, &branding.Layout, &branding.Contact, &branding.SocialLinks,
			&branding.CreatedAt, &branding.UpdatedAt,
		)
	})

	if orgID != nil {
		branding.OrganizationID = *orgID
	}
	if pageIDPtr != nil {
		branding.LandingPageID = pageIDPtr
	}

	return branding, err
}

func (r *resolverRepository) ResolveVersions(ctx context.Context, scope coretenant.Scope, pageID string) ([]domain.LandingPageVersion, error) {
	query := `
		SELECT
			id, organization_id, landing_page_id, version,
			change_note, snapshot, created_by, created_at
		FROM landing_page_versions
		WHERE landing_page_id = $1
		ORDER BY version DESC
	`

	var versions []domain.LandingPageVersion

	err := r.within(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, pageID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var v domain.LandingPageVersion
			var createdBy *string
			err := rows.Scan(
				&v.ID, &v.OrganizationID, &v.LandingPageID, &v.Version,
				&v.ChangeNote, &v.Snapshot, &createdBy, &v.CreatedAt,
			)
			if err != nil {
				return err
			}
			if createdBy != nil {
				v.CreatedBy = *createdBy
			}
			versions = append(versions, v)
		}

		return rows.Err()
	})

	return versions, err
}

func (r *resolverRepository) ResolveMenus(ctx context.Context, scope coretenant.Scope) ([]domain.LandingMenu, error) {
	query := `
		SELECT
			id, name, location, is_active, created_at, updated_at
		FROM landing_menus
		WHERE organization_id = $1
			AND is_active = true
			AND deleted_at IS NULL
		ORDER BY location ASC, created_at ASC
	`

	var menus []domain.LandingMenu

	err := r.within(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID())
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var menu domain.LandingMenu
			err := rows.Scan(
				&menu.ID, &menu.Name, &menu.Location, &menu.IsActive,
				&menu.CreatedAt, &menu.UpdatedAt,
			)
			if err != nil {
				return err
			}
			menu.OrganizationID = scope.OrganizationID()
			menus = append(menus, menu)
		}

		return rows.Err()
	})

	return menus, err
}

func (r *resolverRepository) ResolveMenuItems(ctx context.Context, scope coretenant.Scope, menuID string) ([]domain.LandingMenuItem, error) {
	query := `
		SELECT
			id, menu_id, parent_id, label, link_type, destination, target,
			sort_order, is_enabled, created_at, updated_at
		FROM landing_menu_items
		WHERE organization_id = $1
			AND menu_id = $2
			AND is_enabled = true
		ORDER BY COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), sort_order ASC
	`

	var items []domain.LandingMenuItem

	err := r.within(ctx, scope, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, scope.OrganizationID(), menuID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var item domain.LandingMenuItem
			var parentID *string
			err := rows.Scan(
				&item.ID, &item.MenuID, &parentID, &item.Label, &item.LinkType,
				&item.Destination, &item.Target, &item.SortOrder, &item.IsEnabled,
				&item.CreatedAt, &item.UpdatedAt,
			)
			if err != nil {
				return err
			}
			item.OrganizationID = scope.OrganizationID()
			if parentID != nil {
				item.ParentID = parentID
			}
			items = append(items, item)
		}

		return rows.Err()
	})

	return items, err
}

func (r *resolverRepository) within(
	ctx context.Context,
	scope coretenant.Scope,
	fn func(pgx.Tx) error,
) error {
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
