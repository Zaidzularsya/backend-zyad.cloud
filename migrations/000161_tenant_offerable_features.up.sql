-- Catalog markers on the feature registry: which features a tenant may offer in its own
-- products (tenant_offerable) and, for integer features, the maximum a tenant may sell
-- (tenant_max). Defaults keep every existing feature non-offerable, so no behavior changes.
ALTER TABLE product_features
	ADD COLUMN tenant_offerable boolean NOT NULL DEFAULT false,
	ADD COLUMN tenant_max integer NULL,
	ADD CONSTRAINT product_features_tenant_max_positive CHECK (tenant_max IS NULL OR tenant_max > 0),
	ADD CONSTRAINT product_features_tenant_offerable_integer_max CHECK (NOT (tenant_offerable AND value_type = 'integer' AND tenant_max IS NULL));

INSERT INTO product_features (
	feature_key, module, name, description, value_type, unit, reset_strategy, is_active, tenant_offerable, created_at, updated_at
)
VALUES
	('landing.page_activation', 'landing', 'Landing page activation', 'Allows a workspace to activate and publish its landing page', 'boolean', NULL, 'never', true, true, now(), now())
ON CONFLICT (feature_key)
DO NOTHING;
