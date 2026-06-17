CREATE TABLE IF NOT EXISTS landing_section_templates (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	name varchar(200) NOT NULL,
	description text,
	section_type varchar(50) NOT NULL,
	content jsonb NOT NULL DEFAULT '{}'::jsonb,
	style jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT landing_section_templates_type_check CHECK (
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
	CONSTRAINT landing_section_templates_content_object_check CHECK (jsonb_typeof(content) = 'object'),
	CONSTRAINT landing_section_templates_style_object_check CHECK (jsonb_typeof(style) = 'object'),
	CONSTRAINT landing_section_templates_name_not_blank_check CHECK (char_length(btrim(name)) > 0),
	CONSTRAINT fk_landing_section_templates_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_section_templates_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_landing_section_templates_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_landing_section_templates_deleted_at
	ON landing_section_templates(deleted_at);

CREATE INDEX IF NOT EXISTS idx_landing_section_templates_org_type
	ON landing_section_templates(organization_id, section_type)
	WHERE deleted_at IS NULL;


CREATE TABLE IF NOT EXISTS landing_ctas (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	name varchar(200) NOT NULL,
	label varchar(100) NOT NULL,
	type varchar(50) NOT NULL,
	target varchar(30) NOT NULL DEFAULT 'self',
	destination text NOT NULL,
	tracking_key varchar(120) NOT NULL,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT landing_ctas_type_check CHECK (
		type IN (
			'contact_form',
			'whatsapp',
			'external_link',
			'internal_page',
			'document_download'
		)
	),
	CONSTRAINT landing_ctas_target_check CHECK (target IN ('self', 'new_tab')),
	CONSTRAINT landing_ctas_name_not_blank_check CHECK (char_length(btrim(name)) > 0),
	CONSTRAINT landing_ctas_label_not_blank_check CHECK (char_length(btrim(label)) > 0),
	CONSTRAINT landing_ctas_tracking_key_format_check CHECK (
		tracking_key = lower(tracking_key)
		AND tracking_key ~ '^[a-z0-9]([a-z0-9-]{0,118}[a-z0-9])?$'
	),
	CONSTRAINT fk_landing_ctas_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_ctas_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_landing_ctas_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_ctas_org_tracking_key_active_unique
	ON landing_ctas(organization_id, lower(tracking_key))
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_landing_ctas_deleted_at
	ON landing_ctas(deleted_at);


CREATE TABLE IF NOT EXISTS landing_menus (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	name varchar(200) NOT NULL,
	location varchar(50) NOT NULL,
	is_active boolean NOT NULL DEFAULT true,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT landing_menus_name_not_blank_check CHECK (char_length(btrim(name)) > 0),
	CONSTRAINT landing_menus_location_check CHECK (location IN ('header', 'footer', 'sidebar')),
	CONSTRAINT fk_landing_menus_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_menus_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_landing_menus_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_menus_org_location_active_unique
	ON landing_menus(organization_id, location)
	WHERE is_active = true AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_landing_menus_deleted_at
	ON landing_menus(deleted_at);


CREATE TABLE IF NOT EXISTS landing_menu_items (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	menu_id uuid NOT NULL,
	parent_id uuid,
	label varchar(100) NOT NULL,
	link_type varchar(30) NOT NULL,
	destination text NOT NULL,
	target varchar(30) NOT NULL DEFAULT 'self',
	sort_order integer NOT NULL,
	is_enabled boolean NOT NULL DEFAULT true,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT landing_menu_items_link_type_check CHECK (
		link_type IN ('internal_page', 'external_link', 'anchor', 'button')
	),
	CONSTRAINT landing_menu_items_target_check CHECK (target IN ('self', 'new_tab')),
	CONSTRAINT landing_menu_items_sort_order_check CHECK (sort_order >= 0),
	CONSTRAINT landing_menu_items_label_not_blank_check CHECK (char_length(btrim(label)) > 0),
	CONSTRAINT fk_landing_menu_items_menu
		FOREIGN KEY (organization_id, menu_id)
		REFERENCES landing_menus(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_menu_items_parent
		FOREIGN KEY (parent_id)
		REFERENCES landing_menu_items(id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_menu_items_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_menu_items_menu_sort_unique
	ON landing_menu_items(organization_id, menu_id, COALESCE(parent_id, '00000000-0000-0000-0000-000000000000'::uuid), sort_order);

SELECT apply_organization_rls('landing_section_templates'::regclass);
SELECT apply_organization_rls('landing_ctas'::regclass);
SELECT apply_organization_rls('landing_menus'::regclass);
SELECT apply_organization_rls('landing_menu_items'::regclass);
