-- Number of WhatsApp sessions (connected numbers) an organization may create.
-- free/starter have no whatsapp.enabled, so they get no row. Organizations
-- whose plan was synced before this feature existed have no runtime
-- entitlement row yet; the whatsapp SessionService falls back to a limit of 1
-- until the next plan sync (docs/reference-whatsapp.md).
INSERT INTO product_features (
	feature_key,
	module,
	name,
	description,
	value_type,
	unit,
	reset_strategy,
	is_active,
	created_at,
	updated_at
)
VALUES (
	'whatsapp.max_sessions',
	'whatsapp',
	'Maximum WhatsApp sessions',
	'Maximum number of connected WhatsApp numbers (WAHA sessions)',
	'integer',
	'session',
	'never',
	true,
	now(),
	now()
)
ON CONFLICT (feature_key)
DO UPDATE SET
	module = EXCLUDED.module,
	name = EXCLUDED.name,
	description = EXCLUDED.description,
	value_type = EXCLUDED.value_type,
	unit = EXCLUDED.unit,
	reset_strategy = EXCLUDED.reset_strategy,
	is_active = EXCLUDED.is_active,
	updated_at = now();

WITH plan_entitlements_seed(plan_code, value_int) AS (
	VALUES
		('growth', 1::bigint),
		('business', 3::bigint),
		('enterprise', 10::bigint)
),
plans AS (
	SELECT id, code
	FROM product_plans
	WHERE deleted_at IS NULL
),
features AS (
	SELECT id
	FROM product_features
	WHERE feature_key = 'whatsapp.max_sessions'
)
INSERT INTO product_plan_entitlements (
	plan_id,
	feature_id,
	value_bool,
	value_int,
	value_decimal,
	value_string,
	limits,
	created_at,
	updated_at
)
SELECT
	plans.id,
	features.id,
	NULL::boolean,
	seed.value_int,
	NULL::numeric,
	NULL::text,
	'{}'::jsonb,
	now(),
	now()
FROM plan_entitlements_seed seed
JOIN plans ON plans.code = seed.plan_code
CROSS JOIN features
ON CONFLICT (plan_id, feature_id)
DO UPDATE SET
	value_int = EXCLUDED.value_int,
	updated_at = now();
