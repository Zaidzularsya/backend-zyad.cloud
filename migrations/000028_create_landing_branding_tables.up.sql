CREATE TABLE IF NOT EXISTS landing_brandings (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	landing_page_id uuid,
	company_name varchar(200),
	tagline varchar(255),
	logo_light_url text,
	logo_dark_url text,
	favicon_url text,
	social_image_url text,
	colors jsonb NOT NULL DEFAULT '{}'::jsonb,
	typography jsonb NOT NULL DEFAULT '{}'::jsonb,
	shape jsonb NOT NULL DEFAULT '{}'::jsonb,
	layout jsonb NOT NULL DEFAULT '{}'::jsonb,
	contact jsonb NOT NULL DEFAULT '{}'::jsonb,
	social_links jsonb NOT NULL DEFAULT '[]'::jsonb,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT landing_brandings_colors_object_check CHECK (jsonb_typeof(colors) = 'object'),
	CONSTRAINT landing_brandings_typography_object_check CHECK (jsonb_typeof(typography) = 'object'),
	CONSTRAINT landing_brandings_shape_object_check CHECK (jsonb_typeof(shape) = 'object'),
	CONSTRAINT landing_brandings_layout_object_check CHECK (jsonb_typeof(layout) = 'object'),
	CONSTRAINT landing_brandings_contact_object_check CHECK (jsonb_typeof(contact) = 'object'),
	CONSTRAINT landing_brandings_social_links_array_check CHECK (jsonb_typeof(social_links) = 'array'),
	CONSTRAINT fk_landing_brandings_page
		FOREIGN KEY (organization_id, landing_page_id)
		REFERENCES landing_pages(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_brandings_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_brandings_org_default
	ON landing_brandings(organization_id)
	WHERE landing_page_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_brandings_page_override
	ON landing_brandings(organization_id, landing_page_id)
	WHERE landing_page_id IS NOT NULL;


CREATE TABLE IF NOT EXISTS landing_domain_bindings (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	organization_domain_id uuid NOT NULL,
	landing_page_id uuid NOT NULL,
	is_primary boolean NOT NULL DEFAULT false,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT fk_landing_domain_bindings_page
		FOREIGN KEY (organization_id, landing_page_id)
		REFERENCES landing_pages(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_domain_bindings_domain
		FOREIGN KEY (organization_domain_id)
		REFERENCES organization_domains(id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_domain_bindings_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_domain_bindings_domain_unique
	ON landing_domain_bindings(organization_domain_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_domain_bindings_page_primary_unique
	ON landing_domain_bindings(organization_id, landing_page_id)
	WHERE is_primary = true;

SELECT apply_organization_rls('landing_brandings'::regclass);
SELECT apply_organization_rls('landing_domain_bindings'::regclass);
