ALTER TABLE catalog_products
	ADD COLUMN IF NOT EXISTS is_public boolean NOT NULL DEFAULT false,
	ADD COLUMN IF NOT EXISTS listing_code varchar(50),
	ADD COLUMN IF NOT EXISTS listing_order integer NOT NULL DEFAULT 0;

ALTER TABLE catalog_products
	ADD CONSTRAINT catalog_products_listing_check
		CHECK (is_public = false OR (listing_code IS NOT NULL AND category_id IS NOT NULL)),
	ADD CONSTRAINT catalog_products_listing_code_format
		CHECK (listing_code IS NULL OR listing_code ~ '^[a-z0-9][a-z0-9-]{0,49}$');

CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_products_listing_unique
	ON catalog_products(organization_id, listing_code, billing_frequency)
	WHERE is_public AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS catalog_product_features (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	product_id uuid NOT NULL,
	feature_key varchar(100) NOT NULL,
	value jsonb NOT NULL,
	display_label varchar(200),
	position integer NOT NULL DEFAULT 0,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT catalog_product_features_org_id_unique UNIQUE (organization_id, id),
	CONSTRAINT catalog_product_features_product_key_unique UNIQUE (organization_id, product_id, feature_key),
	CONSTRAINT fk_catalog_product_features_org
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_catalog_product_features_product
		FOREIGN KEY (organization_id, product_id)
		REFERENCES catalog_products(organization_id, id) ON DELETE CASCADE
);
SELECT apply_organization_rls('catalog_product_features'::regclass);

ALTER TABLE crm_quotation_items
	ADD COLUMN IF NOT EXISTS features jsonb NOT NULL DEFAULT '[]'::jsonb;
