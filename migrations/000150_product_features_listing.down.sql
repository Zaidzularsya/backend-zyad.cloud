ALTER TABLE crm_quotation_items DROP COLUMN IF EXISTS features;

DROP TABLE IF EXISTS catalog_product_features;

DROP INDEX IF EXISTS idx_catalog_products_listing_unique;
ALTER TABLE catalog_products
	DROP CONSTRAINT IF EXISTS catalog_products_listing_code_format,
	DROP CONSTRAINT IF EXISTS catalog_products_listing_check,
	DROP COLUMN IF EXISTS listing_order,
	DROP COLUMN IF EXISTS listing_code,
	DROP COLUMN IF EXISTS is_public;
