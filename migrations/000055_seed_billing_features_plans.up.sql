WITH billing_features_seed(
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
INSERT INTO billing_features (
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
FROM billing_features_seed
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

WITH billing_plans_seed(
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
INSERT INTO billing_plans (
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
FROM billing_plans_seed
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

WITH billing_plan_prices_seed(
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
	FROM billing_plans
	WHERE deleted_at IS NULL
)
INSERT INTO billing_plan_prices (
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
FROM billing_plan_prices_seed seed
JOIN plans ON plans.code = seed.plan_code
ON CONFLICT (plan_id, billing_interval, currency)
WHERE deleted_at IS NULL
DO UPDATE SET
	amount = EXCLUDED.amount,
	is_active = EXCLUDED.is_active,
	updated_at = now();
