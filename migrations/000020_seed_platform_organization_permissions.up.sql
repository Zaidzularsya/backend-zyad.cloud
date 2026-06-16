WITH platform_organization_permissions(permission_name, description) AS (
	VALUES
		('platform.organization.read', 'Read platform organization registry'),
		('platform.organization.manage', 'Create and update organizations'),
		('platform.organization.suspend', 'Change organization lifecycle status'),
		('platform.organization.provision', 'Provision or retry organization provisioning')
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
		'platform_organization',
		split_part(permission_name, '.', 3),
		permission_name,
		permission_name,
		description,
		now(),
		now()
	FROM platform_organization_permissions
	ON CONFLICT (permission_name)
	DO UPDATE SET
		module = EXCLUDED.module,
		action = EXCLUDED.action,
		name = EXCLUDED.name,
		slug = EXCLUDED.slug,
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
	upserted_permissions.id,
	'all',
	now()
FROM super_admin_role
CROSS JOIN upserted_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;
