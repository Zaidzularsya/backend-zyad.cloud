WITH finance_permissions(permission_name, action, description) AS (
	VALUES
		('platform.finance.tax.read', 'tax_read', 'Read finance tax types, rates, transactions, and summary'),
		('platform.finance.tax.manage', 'tax_manage', 'Manage finance tax rates and record tax transactions')
),
upserted_permissions AS (
	INSERT INTO permissions (
		permission_name,
		module,
		action,
		name,
		slug,
		description,
		created_at,
		updated_at
	)
	SELECT
		permission_name,
		'finance',
		action,
		permission_name,
		permission_name,
		description,
		now(),
		now()
	FROM finance_permissions
	ON CONFLICT (slug)
	DO UPDATE SET
		permission_name = EXCLUDED.permission_name,
		module = EXCLUDED.module,
		action = EXCLUDED.action,
		name = EXCLUDED.name,
		description = EXCLUDED.description,
		updated_at = now()
	RETURNING id, permission_name
),
platform_roles AS (
	SELECT id
	FROM roles
	WHERE role_name = 'super_admin'
		OR slug = 'super_admin'
)
INSERT INTO role_permissions (
	role_id,
	permission_id,
	scope,
	granted_at
)
SELECT
	platform_roles.id,
	upserted_permissions.id,
	'all',
	now()
FROM platform_roles
CROSS JOIN upserted_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;
