WITH seeded_plans AS (
	SELECT id
	FROM product_plans
	WHERE code IN ('free', 'starter', 'growth', 'business', 'enterprise')
),
seeded_features AS (
	SELECT id
	FROM product_features
	WHERE feature_key IN (
		'users.max_users',
		'users.invite_user',
		'roles.custom_roles',
		'landing.enabled',
		'landing.max_pages',
		'landing.max_sections_per_page',
		'landing.custom_domain',
		'landing.remove_branding',
		'landing.analytics',
		'crm.enabled',
		'crm.max_contacts',
		'crm.import_export',
		'crm.pipeline',
		'media.enabled',
		'media.max_storage_mb',
		'media.max_file_size_mb',
		'domain.enabled',
		'domain.max_custom_domains',
		'domain.ssl_auto_provision',
		'whatsapp.enabled',
		'whatsapp.max_messages_per_month',
		'pos.enabled',
		'pos.max_products',
		'membership.enabled',
		'membership.max_members',
		'automation.enabled',
		'automation.max_runs_per_month'
	)
)
DELETE FROM product_plan_entitlements
WHERE plan_id IN (SELECT id FROM seeded_plans)
	AND feature_id IN (SELECT id FROM seeded_features);

WITH seeded_plans AS (
	SELECT id
	FROM product_plans
	WHERE code IN ('free', 'starter', 'growth', 'business', 'enterprise')
)
DELETE FROM product_plan_prices
WHERE plan_id IN (SELECT id FROM seeded_plans);

DELETE FROM product_plans
WHERE code IN ('free', 'starter', 'growth', 'business', 'enterprise');

DELETE FROM product_features
WHERE feature_key IN (
	'users.max_users',
	'users.invite_user',
	'roles.custom_roles',
	'landing.enabled',
	'landing.max_pages',
	'landing.max_sections_per_page',
	'landing.custom_domain',
	'landing.remove_branding',
	'landing.analytics',
	'crm.enabled',
	'crm.max_contacts',
	'crm.import_export',
	'crm.pipeline',
	'media.enabled',
	'media.max_storage_mb',
	'media.max_file_size_mb',
	'domain.enabled',
	'domain.max_custom_domains',
	'domain.ssl_auto_provision',
	'whatsapp.enabled',
	'whatsapp.max_messages_per_month',
	'pos.enabled',
	'pos.max_products',
	'membership.enabled',
	'membership.max_members',
	'automation.enabled',
	'automation.max_runs_per_month'
);
