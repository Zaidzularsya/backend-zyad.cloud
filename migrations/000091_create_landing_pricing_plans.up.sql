CREATE TABLE IF NOT EXISTS landing_pricing_plans (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	name varchar(100) NOT NULL,
	price_label varchar(100) NOT NULL,
	interval_label varchar(50),
	description text,
	features jsonb NOT NULL DEFAULT '[]'::jsonb,
	cta_label varchar(100) NOT NULL DEFAULT 'Pilih paket',
	cta_url text,
	is_featured boolean NOT NULL DEFAULT false,
	sort_order integer NOT NULL DEFAULT 0,
	is_enabled boolean NOT NULL DEFAULT true,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT landing_pricing_plans_name_not_blank_check CHECK (char_length(btrim(name)) > 0),
	CONSTRAINT landing_pricing_plans_price_label_not_blank_check CHECK (char_length(btrim(price_label)) > 0),
	CONSTRAINT landing_pricing_plans_features_array_check CHECK (jsonb_typeof(features) = 'array'),
	CONSTRAINT landing_pricing_plans_sort_order_check CHECK (sort_order >= 0),
	CONSTRAINT fk_landing_pricing_plans_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_pricing_plans_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_landing_pricing_plans_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_landing_pricing_plans_org_sort
	ON landing_pricing_plans(organization_id, sort_order)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_landing_pricing_plans_deleted_at
	ON landing_pricing_plans(deleted_at);

SELECT apply_organization_rls('landing_pricing_plans'::regclass);
