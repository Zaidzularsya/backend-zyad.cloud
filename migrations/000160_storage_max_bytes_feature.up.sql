-- Registers storage.max_bytes, the key the asset module enforces for tenant storage
-- (internal/modules/asset). Until now it existed only as ad-hoc entitlement rows, so it
-- could not be part of a catalog product (the FREE plan). Registering the key alone
-- changes no behavior; values come from product features / entitlements.
INSERT INTO product_features (
	feature_key, module, name, description, value_type, unit, reset_strategy, is_active, created_at, updated_at
)
VALUES
	('storage.max_bytes', 'storage', 'Maximum tenant storage', 'Maximum total tenant storage in bytes (enforced by the asset module)', 'integer', 'byte', 'never', true, now(), now())
ON CONFLICT (feature_key)
DO NOTHING;
