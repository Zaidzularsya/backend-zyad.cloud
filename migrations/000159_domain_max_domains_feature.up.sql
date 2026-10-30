-- Total domain cap (subdomain + custom) per organization. Plan values are
-- assigned separately; registering the key alone changes no behavior.
INSERT INTO product_features (
	feature_key, module, name, description, value_type, unit, reset_strategy, is_active, created_at, updated_at
)
VALUES
	('domain.max_domains', 'domain', 'Maximum domains', 'Maximum number of domains (subdomain and custom) an organization may hold', 'integer', 'domain', 'never', true, now(), now())
ON CONFLICT (feature_key)
DO NOTHING;
