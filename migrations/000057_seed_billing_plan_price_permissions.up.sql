WITH plan_price_permissions(permission_name, action, description, permission_scope) AS (
	VALUES
		('platform.billing.plan_price.read', 'plan_price_read', 'Read billing plan prices', 'all'),
		('platform.billing.plan_price.manage', 'plan_price_manage', 'Manage billing plan prices', 'all')
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
		'billing',
		action,
		permission_name,
		permission_name,
		description,
		now(),
		now()
	FROM plan_price_permissions
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
),
platform_permissions AS (
	SELECT id
	FROM upserted_permissions
)
INSERT INTO role_permissions (
	role_id,
	permission_id,
	scope,
	granted_at
)
SELECT
	platform_roles.id,
	platform_permissions.id,
	'all',
	now()
FROM platform_roles
CROSS JOIN platform_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;
