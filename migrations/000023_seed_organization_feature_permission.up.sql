WITH organization_feature_permission(permission_name, action, description) AS (
	VALUES (
		'organization.feature.read',
		'feature_read',
		'Read current organization features and usage'
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
		'organization',
		action,
		permission_name,
		permission_name,
		description,
		now(),
		now()
	FROM organization_feature_permission
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
	'organization',
	now()
FROM super_admin_role
CROSS JOIN upserted_permission
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;
