CREATE TABLE IF NOT EXISTS catalog_product_categories (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	name varchar(100) NOT NULL,
	position integer NOT NULL DEFAULT 0,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT catalog_product_categories_org_id_unique UNIQUE (organization_id, id),
	CONSTRAINT catalog_product_categories_name_not_blank CHECK (char_length(btrim(name)) > 0),
	CONSTRAINT fk_catalog_product_categories_org
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_categories_org_name_unique
	ON catalog_product_categories(organization_id, lower(name)) WHERE deleted_at IS NULL;
SELECT apply_organization_rls('catalog_product_categories'::regclass);

CREATE TABLE IF NOT EXISTS catalog_products (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	category_id uuid,
	sku varchar(64),
	name varchar(200) NOT NULL,
	description text,
	unit varchar(30) NOT NULL DEFAULT 'pcs',
	base_price numeric(18, 2) NOT NULL DEFAULT 0,
	tax_percent numeric(5, 2) NOT NULL DEFAULT 0,
	currency char(3) NOT NULL DEFAULT 'IDR',
	is_active boolean NOT NULL DEFAULT true,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT catalog_products_org_id_unique UNIQUE (organization_id, id),
	CONSTRAINT catalog_products_name_not_blank CHECK (char_length(btrim(name)) > 0),
	CONSTRAINT catalog_products_unit_not_blank CHECK (char_length(btrim(unit)) > 0),
	CONSTRAINT catalog_products_price_check CHECK (base_price >= 0),
	CONSTRAINT catalog_products_tax_check CHECK (tax_percent >= 0 AND tax_percent <= 100),
	CONSTRAINT fk_catalog_products_org
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_catalog_products_category
		FOREIGN KEY (organization_id, category_id)
		REFERENCES catalog_product_categories(organization_id, id) ON DELETE SET NULL (category_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_products_org_sku_unique
	ON catalog_products(organization_id, lower(sku)) WHERE deleted_at IS NULL AND sku IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_catalog_products_org_active
	ON catalog_products(organization_id, is_active) WHERE deleted_at IS NULL;
SELECT apply_organization_rls('catalog_products'::regclass);
