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
	'crm.lead_form',
	'crm',
	'CRM lead capture form',
	'Allow CRM lead capture from public forms and integrations',
	'boolean',
	NULL,
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

WITH plan_entitlements_seed(plan_code, value_bool) AS (
	VALUES
		('free', false),
		('starter', false),
		('growth', true),
		('business', true),
		('enterprise', true)
),
plans AS (
	SELECT id, code
	FROM product_plans
	WHERE deleted_at IS NULL
),
features AS (
	SELECT id
	FROM product_features
	WHERE feature_key = 'crm.lead_form'
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
	seed.value_bool,
	NULL::bigint,
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
	value_bool = EXCLUDED.value_bool,
	updated_at = now();
