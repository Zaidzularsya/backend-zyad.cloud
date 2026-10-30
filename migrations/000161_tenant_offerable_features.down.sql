-- landing.page_activation is not referenced by any entitlement yet, so the row can go.
DELETE FROM product_features
WHERE feature_key = 'landing.page_activation';

ALTER TABLE product_features
	DROP CONSTRAINT IF EXISTS product_features_tenant_offerable_integer_max,
	DROP CONSTRAINT IF EXISTS product_features_tenant_max_positive,
	DROP COLUMN IF EXISTS tenant_max,
	DROP COLUMN IF EXISTS tenant_offerable;
