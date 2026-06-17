CREATE TABLE IF NOT EXISTS landing_pages (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	name varchar(200) NOT NULL,
	title varchar(255) NOT NULL,
	slug varchar(120) NOT NULL,
	page_type varchar(50) NOT NULL,
	status varchar(30) NOT NULL DEFAULT 'draft',
	visibility varchar(40) NOT NULL DEFAULT 'public',
	password_hash text,
	locale varchar(20) NOT NULL DEFAULT 'id-ID',
	timezone varchar(100) NOT NULL DEFAULT 'Asia/Jakarta',
	is_homepage boolean NOT NULL DEFAULT false,
	published_version integer NOT NULL DEFAULT 0,
	publish_at timestamp without time zone,
	unpublish_at timestamp without time zone,
	published_at timestamp without time zone,
	settings jsonb NOT NULL DEFAULT '{}'::jsonb,
	seo jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT landing_pages_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT landing_pages_page_type_check CHECK (
		page_type IN (
			'homepage',
			'company_profile',
			'product_service',
			'campaign',
			'pricing',
			'contact',
			'lead_capture',
			'promo_event',
			'portfolio_case_study'
		)
	),
	CONSTRAINT landing_pages_status_check CHECK (
		status IN ('draft', 'published', 'unpublished', 'archived')
	),
	CONSTRAINT landing_pages_visibility_check CHECK (
		visibility IN ('public', 'private', 'password_protected')
	),
	CONSTRAINT landing_pages_slug_format_check CHECK (
		slug = lower(slug)
		AND slug ~ '^[a-z0-9]([a-z0-9-]{0,118}[a-z0-9])?$'
	),
	CONSTRAINT landing_pages_name_not_blank_check CHECK (
		char_length(btrim(name)) > 0
	),
	CONSTRAINT landing_pages_title_not_blank_check CHECK (
		char_length(btrim(title)) > 0
	),
	CONSTRAINT landing_pages_published_version_check CHECK (
		published_version >= 0
	),
	CONSTRAINT landing_pages_password_visibility_check CHECK (
		(visibility = 'password_protected' AND password_hash IS NOT NULL)
		OR (visibility <> 'password_protected' AND password_hash IS NULL)
	),
	CONSTRAINT landing_pages_schedule_window_check CHECK (
		unpublish_at IS NULL
		OR publish_at IS NULL
		OR unpublish_at > publish_at
	),
	CONSTRAINT landing_pages_settings_object_check CHECK (
		jsonb_typeof(settings) = 'object'
	),
	CONSTRAINT landing_pages_seo_object_check CHECK (
		jsonb_typeof(seo) = 'object'
	),
	CONSTRAINT fk_landing_pages_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_pages_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_landing_pages_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_pages_organization_slug_active_unique
	ON landing_pages(organization_id, lower(slug))
	WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_pages_organization_homepage_unique
	ON landing_pages(organization_id)
	WHERE is_homepage = true
		AND status <> 'archived'
		AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_landing_pages_organization_status
	ON landing_pages(organization_id, status)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_landing_pages_organization_type
	ON landing_pages(organization_id, page_type)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_landing_pages_publish_schedule
	ON landing_pages(organization_id, publish_at, unpublish_at)
	WHERE deleted_at IS NULL
		AND (publish_at IS NOT NULL OR unpublish_at IS NOT NULL);

CREATE INDEX IF NOT EXISTS idx_landing_pages_deleted_at
	ON landing_pages(deleted_at);

CREATE TABLE IF NOT EXISTS landing_page_sections (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	landing_page_id uuid NOT NULL,
	section_key varchar(120) NOT NULL,
	section_type varchar(50) NOT NULL,
	name varchar(200) NOT NULL,
	sort_order integer NOT NULL,
	is_enabled boolean NOT NULL DEFAULT true,
	content jsonb NOT NULL DEFAULT '{}'::jsonb,
	style jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT landing_page_sections_type_check CHECK (
		section_type IN (
			'hero',
			'about',
			'features',
			'services',
			'product_showcase',
			'content',
			'gallery',
			'portfolio',
			'testimonial',
			'pricing',
			'faq',
			'cta',
			'form',
			'contact',
			'newsletter',
			'partner_logos',
			'statistics',
			'footer'
		)
	),
	CONSTRAINT landing_page_sections_key_format_check CHECK (
		section_key = lower(section_key)
		AND section_key ~ '^[a-z0-9]([a-z0-9-]{0,118}[a-z0-9])?$'
	),
	CONSTRAINT landing_page_sections_name_not_blank_check CHECK (
		char_length(btrim(name)) > 0
	),
	CONSTRAINT landing_page_sections_sort_order_check CHECK (
		sort_order >= 0
	),
	CONSTRAINT landing_page_sections_content_object_check CHECK (
		jsonb_typeof(content) = 'object'
	),
	CONSTRAINT landing_page_sections_style_object_check CHECK (
		jsonb_typeof(style) = 'object'
	),
	CONSTRAINT fk_landing_page_sections_page
		FOREIGN KEY (organization_id, landing_page_id)
		REFERENCES landing_pages(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_page_sections_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_page_sections_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_landing_page_sections_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_page_sections_page_key_active_unique
	ON landing_page_sections(organization_id, landing_page_id, lower(section_key))
	WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_page_sections_page_sort_active_unique
	ON landing_page_sections(organization_id, landing_page_id, sort_order)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_landing_page_sections_page_enabled
	ON landing_page_sections(organization_id, landing_page_id, is_enabled, sort_order)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_landing_page_sections_organization_type
	ON landing_page_sections(organization_id, section_type)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_landing_page_sections_deleted_at
	ON landing_page_sections(deleted_at);

SELECT apply_organization_rls('landing_pages'::regclass);
SELECT apply_organization_rls('landing_page_sections'::regclass);
