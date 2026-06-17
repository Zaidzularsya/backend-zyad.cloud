WITH platform_impersonation_permission(permission_name, action, description) AS (
	VALUES (
		'platform.organization.impersonate',
		'organization_impersonate',
		'Start and stop operator impersonation for customer organizations'
	)
),
upserted_permission AS (
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
		'platform',
		action,
		permission_name,
		permission_name,
		description,
		now(),
		now()
	FROM platform_impersonation_permission
	ON CONFLICT (slug)
	DO UPDATE SET
		permission_name = EXCLUDED.permission_name,
		module = EXCLUDED.module,
		action = EXCLUDED.action,
		name = EXCLUDED.name,
		description = EXCLUDED.description,
		updated_at = now()
	RETURNING id
),
super_admin_role AS (
	SELECT id
	FROM roles
	WHERE role_name = 'super_admin'
		OR slug = 'super_admin'
	ORDER BY created_at
	LIMIT 1
)
INSERT INTO role_permissions (
	role_id,
	permission_id,
	scope,
	granted_at
)
SELECT
	super_admin_role.id,
	upserted_permission.id,
	'all',
	now()
FROM super_admin_role
CROSS JOIN upserted_permission
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;
