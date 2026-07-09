WITH product_features_seed(
	feature_key,
	module,
	name,
	description,
	value_type,
	unit,
	reset_strategy
) AS (
	VALUES
		('users.max_users', 'users', 'Maximum users', 'Maximum active users in an organization', 'integer', 'user', 'never'),
		('users.invite_user', 'users', 'Invite users', 'Allow inviting users into an organization', 'boolean', NULL, 'never'),
		('roles.custom_roles', 'roles', 'Custom roles', 'Allow custom organization roles', 'boolean', NULL, 'never'),
		('landing.enabled', 'landing', 'Landing page module', 'Enable Landing Page capability', 'boolean', NULL, 'never'),
		('landing.max_pages', 'landing', 'Maximum landing pages', 'Maximum active landing pages', 'integer', 'page', 'never'),
		('landing.max_sections_per_page', 'landing', 'Maximum sections per landing page', 'Maximum sections on a landing page', 'integer', 'section', 'never'),
		('landing.custom_domain', 'landing', 'Landing custom domain', 'Allow binding custom domains to landing pages', 'boolean', NULL, 'never'),
		('landing.remove_branding', 'landing', 'Remove Zyad branding', 'Allow removing Zyad branding from landing pages', 'boolean', NULL, 'never'),
		('landing.analytics', 'landing', 'Landing analytics', 'Enable landing page analytics dashboard', 'boolean', NULL, 'never'),
		('crm.enabled', 'crm', 'CRM module', 'Enable CRM capability', 'boolean', NULL, 'never'),
		('crm.max_contacts', 'crm', 'Maximum CRM contacts', 'Maximum CRM contacts', 'integer', 'contact', 'never'),
		('crm.import_export', 'crm', 'CRM import export', 'Allow CRM import and export', 'boolean', NULL, 'never'),
		('crm.pipeline', 'crm', 'CRM pipeline', 'Enable CRM pipeline capability', 'boolean', NULL, 'never'),
		('media.enabled', 'media', 'Media module', 'Enable media manager capability', 'boolean', NULL, 'never'),
		('media.max_storage_mb', 'media', 'Maximum media storage', 'Maximum media storage in MB', 'integer', 'MB', 'never'),
		('media.max_file_size_mb', 'media', 'Maximum media file size', 'Maximum single media file size in MB', 'integer', 'MB', 'never'),
		('domain.enabled', 'domain', 'Domain module', 'Enable organization domain capability', 'boolean', NULL, 'never'),
		('domain.max_custom_domains', 'domain', 'Maximum custom domains', 'Maximum active custom domains', 'integer', 'domain', 'never'),
		('domain.ssl_auto_provision', 'domain', 'Automatic SSL provisioning', 'Allow automatic SSL provisioning', 'boolean', NULL, 'never'),
		('whatsapp.enabled', 'whatsapp', 'WhatsApp module', 'Enable WhatsApp capability', 'boolean', NULL, 'never'),
		('whatsapp.max_messages_per_month', 'whatsapp', 'Maximum WhatsApp messages per month', 'Maximum WhatsApp messages per month', 'integer', 'message', 'monthly'),
		('pos.enabled', 'pos', 'POS module', 'Enable POS capability', 'boolean', NULL, 'never'),
		('pos.max_products', 'pos', 'Maximum POS products', 'Maximum POS products', 'integer', 'product', 'never'),
		('membership.enabled', 'membership', 'Membership module', 'Enable Membership capability', 'boolean', NULL, 'never'),
		('membership.max_members', 'membership', 'Maximum members', 'Maximum membership members', 'integer', 'member', 'never'),
		('automation.enabled', 'automation', 'Automation module', 'Enable automation capability', 'boolean', NULL, 'never'),
		('automation.max_runs_per_month', 'automation', 'Maximum automation runs per month', 'Maximum automation runs per month', 'integer', 'run', 'monthly')
)
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
SELECT
	feature_key,
	module,
	name,
	description,
	value_type,
	unit,
	reset_strategy,
	true,
	now(),
	now()
FROM product_features_seed
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

WITH product_plans_seed(
	code,
	name,
	description,
	plan_type,
	is_public,
	is_active,
	sort_order
) AS (
	VALUES
		('free', 'Free', 'Free starter plan for trial and very small organizations', 'free', true, true, 10),
		('starter', 'Starter', 'Starter SaaS plan for small teams', 'paid', true, true, 20),
		('growth', 'Growth', 'Growth SaaS plan for growing businesses', 'paid', true, true, 30),
		('business', 'Business', 'Business SaaS plan for advanced operations', 'paid', true, true, 40),
		('enterprise', 'Enterprise', 'Enterprise custom plan', 'enterprise', false, true, 50)
)
INSERT INTO product_plans (
	code,
	name,
	description,
	plan_type,
	is_public,
	is_active,
	sort_order,
	metadata,
	created_at,
	updated_at
)
SELECT
	code,
	name,
	description,
	plan_type,
	is_public,
	is_active,
	sort_order,
	'{}'::jsonb,
	now(),
	now()
FROM product_plans_seed
ON CONFLICT (code)
WHERE deleted_at IS NULL
DO UPDATE SET
	name = EXCLUDED.name,
	description = EXCLUDED.description,
	plan_type = EXCLUDED.plan_type,
	is_public = EXCLUDED.is_public,
	is_active = EXCLUDED.is_active,
	sort_order = EXCLUDED.sort_order,
	updated_at = now();

WITH product_plan_prices_seed(
	plan_code,
	billing_interval,
	currency,
	amount,
	is_active
) AS (
	VALUES
		('free', 'monthly', 'IDR', 0::numeric, true),
		('starter', 'monthly', 'IDR', 99000::numeric, true),
		('starter', 'yearly', 'IDR', 990000::numeric, true),
		('growth', 'monthly', 'IDR', 299000::numeric, true),
		('growth', 'yearly', 'IDR', 2990000::numeric, true),
		('business', 'monthly', 'IDR', 799000::numeric, true),
		('business', 'yearly', 'IDR', 7990000::numeric, true),
		('enterprise', 'custom', 'IDR', 0::numeric, true)
),
plans AS (
	SELECT id, code
	FROM product_plans
	WHERE deleted_at IS NULL
)
INSERT INTO product_plan_prices (
	plan_id,
	billing_interval,
	currency,
	amount,
	is_active,
	metadata,
	created_at,
	updated_at
)
SELECT
	plans.id,
	seed.billing_interval,
	seed.currency,
	seed.amount,
	seed.is_active,
	'{}'::jsonb,
	now(),
	now()
FROM product_plan_prices_seed seed
JOIN plans ON plans.code = seed.plan_code
ON CONFLICT (plan_id, billing_interval, currency)
WHERE deleted_at IS NULL
DO UPDATE SET
	amount = EXCLUDED.amount,
	is_active = EXCLUDED.is_active,
	updated_at = now();

WITH plan_entitlements_seed(
	plan_code,
	feature_key,
	value_bool,
	value_int,
	value_decimal,
	value_string
) AS (
	VALUES
		('free', 'users.max_users', NULL::boolean, 1::bigint, NULL::numeric, NULL::text),
		('free', 'users.invite_user', false, NULL::bigint, NULL::numeric, NULL::text),
		('free', 'roles.custom_roles', false, NULL::bigint, NULL::numeric, NULL::text),
		('free', 'landing.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('free', 'landing.max_pages', NULL::boolean, 1::bigint, NULL::numeric, NULL::text),
		('free', 'landing.max_sections_per_page', NULL::boolean, 8::bigint, NULL::numeric, NULL::text),
		('free', 'landing.custom_domain', false, NULL::bigint, NULL::numeric, NULL::text),
		('free', 'landing.remove_branding', false, NULL::bigint, NULL::numeric, NULL::text),
		('free', 'landing.analytics', false, NULL::bigint, NULL::numeric, NULL::text),
		('free', 'crm.enabled', false, NULL::bigint, NULL::numeric, NULL::text),
		('free', 'media.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('free', 'media.max_storage_mb', NULL::boolean, 100::bigint, NULL::numeric, NULL::text),
		('free', 'media.max_file_size_mb', NULL::boolean, 5::bigint, NULL::numeric, NULL::text),
		('free', 'domain.enabled', false, NULL::bigint, NULL::numeric, NULL::text),
		('free', 'domain.max_custom_domains', NULL::boolean, 0::bigint, NULL::numeric, NULL::text),
		('free', 'whatsapp.enabled', false, NULL::bigint, NULL::numeric, NULL::text),
		('free', 'pos.enabled', false, NULL::bigint, NULL::numeric, NULL::text),
		('free', 'membership.enabled', false, NULL::bigint, NULL::numeric, NULL::text),
		('free', 'automation.enabled', false, NULL::bigint, NULL::numeric, NULL::text),

		('starter', 'users.max_users', NULL::boolean, 3::bigint, NULL::numeric, NULL::text),
		('starter', 'users.invite_user', true, NULL::bigint, NULL::numeric, NULL::text),
		('starter', 'roles.custom_roles', false, NULL::bigint, NULL::numeric, NULL::text),
		('starter', 'landing.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('starter', 'landing.max_pages', NULL::boolean, 3::bigint, NULL::numeric, NULL::text),
		('starter', 'landing.max_sections_per_page', NULL::boolean, 16::bigint, NULL::numeric, NULL::text),
		('starter', 'landing.custom_domain', false, NULL::bigint, NULL::numeric, NULL::text),
		('starter', 'landing.remove_branding', false, NULL::bigint, NULL::numeric, NULL::text),
		('starter', 'landing.analytics', true, NULL::bigint, NULL::numeric, NULL::text),
		('starter', 'crm.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('starter', 'crm.max_contacts', NULL::boolean, 500::bigint, NULL::numeric, NULL::text),
		('starter', 'crm.import_export', false, NULL::bigint, NULL::numeric, NULL::text),
		('starter', 'crm.pipeline', false, NULL::bigint, NULL::numeric, NULL::text),
		('starter', 'media.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('starter', 'media.max_storage_mb', NULL::boolean, 1000::bigint, NULL::numeric, NULL::text),
		('starter', 'media.max_file_size_mb', NULL::boolean, 10::bigint, NULL::numeric, NULL::text),
		('starter', 'domain.enabled', false, NULL::bigint, NULL::numeric, NULL::text),
		('starter', 'domain.max_custom_domains', NULL::boolean, 0::bigint, NULL::numeric, NULL::text),
		('starter', 'whatsapp.enabled', false, NULL::bigint, NULL::numeric, NULL::text),
		('starter', 'pos.enabled', false, NULL::bigint, NULL::numeric, NULL::text),
		('starter', 'membership.enabled', false, NULL::bigint, NULL::numeric, NULL::text),
		('starter', 'automation.enabled', false, NULL::bigint, NULL::numeric, NULL::text),

		('growth', 'users.max_users', NULL::boolean, 10::bigint, NULL::numeric, NULL::text),
		('growth', 'users.invite_user', true, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'roles.custom_roles', true, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'landing.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'landing.max_pages', NULL::boolean, 10::bigint, NULL::numeric, NULL::text),
		('growth', 'landing.max_sections_per_page', NULL::boolean, 32::bigint, NULL::numeric, NULL::text),
		('growth', 'landing.custom_domain', true, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'landing.remove_branding', false, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'landing.analytics', true, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'crm.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'crm.max_contacts', NULL::boolean, 5000::bigint, NULL::numeric, NULL::text),
		('growth', 'crm.import_export', true, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'crm.pipeline', true, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'media.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'media.max_storage_mb', NULL::boolean, 5000::bigint, NULL::numeric, NULL::text),
		('growth', 'media.max_file_size_mb', NULL::boolean, 25::bigint, NULL::numeric, NULL::text),
		('growth', 'domain.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'domain.max_custom_domains', NULL::boolean, 1::bigint, NULL::numeric, NULL::text),
		('growth', 'domain.ssl_auto_provision', true, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'whatsapp.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'whatsapp.max_messages_per_month', NULL::boolean, 1000::bigint, NULL::numeric, NULL::text),
		('growth', 'pos.enabled', false, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'membership.enabled', false, NULL::bigint, NULL::numeric, NULL::text),
		('growth', 'automation.enabled', false, NULL::bigint, NULL::numeric, NULL::text),

		('business', 'users.max_users', NULL::boolean, 25::bigint, NULL::numeric, NULL::text),
		('business', 'users.invite_user', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'roles.custom_roles', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'landing.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'landing.max_pages', NULL::boolean, 50::bigint, NULL::numeric, NULL::text),
		('business', 'landing.max_sections_per_page', NULL::boolean, 64::bigint, NULL::numeric, NULL::text),
		('business', 'landing.custom_domain', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'landing.remove_branding', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'landing.analytics', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'crm.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'crm.max_contacts', NULL::boolean, 25000::bigint, NULL::numeric, NULL::text),
		('business', 'crm.import_export', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'crm.pipeline', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'media.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'media.max_storage_mb', NULL::boolean, 20000::bigint, NULL::numeric, NULL::text),
		('business', 'media.max_file_size_mb', NULL::boolean, 100::bigint, NULL::numeric, NULL::text),
		('business', 'domain.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'domain.max_custom_domains', NULL::boolean, 5::bigint, NULL::numeric, NULL::text),
		('business', 'domain.ssl_auto_provision', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'whatsapp.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'whatsapp.max_messages_per_month', NULL::boolean, 5000::bigint, NULL::numeric, NULL::text),
		('business', 'pos.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'pos.max_products', NULL::boolean, 5000::bigint, NULL::numeric, NULL::text),
		('business', 'membership.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'membership.max_members', NULL::boolean, 25000::bigint, NULL::numeric, NULL::text),
		('business', 'automation.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('business', 'automation.max_runs_per_month', NULL::boolean, 10000::bigint, NULL::numeric, NULL::text),

		('enterprise', 'users.max_users', NULL::boolean, 1000::bigint, NULL::numeric, NULL::text),
		('enterprise', 'users.invite_user', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'roles.custom_roles', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'landing.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'landing.max_pages', NULL::boolean, 1000::bigint, NULL::numeric, NULL::text),
		('enterprise', 'landing.max_sections_per_page', NULL::boolean, 128::bigint, NULL::numeric, NULL::text),
		('enterprise', 'landing.custom_domain', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'landing.remove_branding', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'landing.analytics', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'crm.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'crm.max_contacts', NULL::boolean, 1000000::bigint, NULL::numeric, NULL::text),
		('enterprise', 'crm.import_export', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'crm.pipeline', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'media.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'media.max_storage_mb', NULL::boolean, 100000::bigint, NULL::numeric, NULL::text),
		('enterprise', 'media.max_file_size_mb', NULL::boolean, 500::bigint, NULL::numeric, NULL::text),
		('enterprise', 'domain.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'domain.max_custom_domains', NULL::boolean, 100::bigint, NULL::numeric, NULL::text),
		('enterprise', 'domain.ssl_auto_provision', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'whatsapp.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'whatsapp.max_messages_per_month', NULL::boolean, 100000::bigint, NULL::numeric, NULL::text),
		('enterprise', 'pos.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'pos.max_products', NULL::boolean, 100000::bigint, NULL::numeric, NULL::text),
		('enterprise', 'membership.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'membership.max_members', NULL::boolean, 1000000::bigint, NULL::numeric, NULL::text),
		('enterprise', 'automation.enabled', true, NULL::bigint, NULL::numeric, NULL::text),
		('enterprise', 'automation.max_runs_per_month', NULL::boolean, 1000000::bigint, NULL::numeric, NULL::text)
),
plans AS (
	SELECT id, code
	FROM product_plans
	WHERE deleted_at IS NULL
),
features AS (
	SELECT id, feature_key
	FROM product_features
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
	seed.value_int,
	seed.value_decimal,
	seed.value_string,
	'{}'::jsonb,
	now(),
	now()
FROM plan_entitlements_seed seed
JOIN plans ON plans.code = seed.plan_code
JOIN features ON features.feature_key = seed.feature_key
ON CONFLICT (plan_id, feature_id)
DO UPDATE SET
	value_bool = EXCLUDED.value_bool,
	value_int = EXCLUDED.value_int,
	value_decimal = EXCLUDED.value_decimal,
	value_string = EXCLUDED.value_string,
	limits = EXCLUDED.limits,
	updated_at = now();
