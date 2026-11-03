-- Permission control center Mikrotik. read: owner + member; update/restore: hanya organization_owner.
WITH mikrotik_control_permissions(permission_name, module, action, description) AS (
	VALUES
		('mikrotik_control.read', 'mikrotik', 'read', 'Read Mikrotik control center settings and readiness'),
		('mikrotik_control.update', 'mikrotik', 'update', 'Update Mikrotik isolation mode, grace days, and pilot accounts'),
		('mikrotik_control.restore', 'mikrotik', 'restore', 'Restore Mikrotik secrets to normal profile')
),
upserted_permissions AS (
	INSERT INTO permissions (permission_name, module, action, name, slug, description, created_at, updated_at)
	SELECT permission_name, module, action, permission_name, permission_name, description, now(), now()
	FROM mikrotik_control_permissions
	ON CONFLICT (slug)
	DO UPDATE SET permission_name = EXCLUDED.permission_name, module = EXCLUDED.module, action = EXCLUDED.action,
		name = EXCLUDED.name, slug = EXCLUDED.slug, description = EXCLUDED.description, updated_at = now()
	RETURNING id
),
owner_roles AS (
	SELECT id
	FROM roles
	WHERE role_name IN ('organization_owner')
		OR slug IN ('organization_owner')
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT owner_roles.id, upserted_permissions.id, 'organization', now()
FROM owner_roles
CROSS JOIN upserted_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET scope = EXCLUDED.scope, granted_at = EXCLUDED.granted_at;

INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT roles.id, permissions.id, 'organization', now()
FROM roles
CROSS JOIN permissions
WHERE (roles.role_name = 'member' OR roles.slug = 'member') AND permissions.permission_name = 'mikrotik_control.read'
ON CONFLICT (role_id, permission_id)
DO UPDATE SET scope = EXCLUDED.scope, granted_at = EXCLUDED.granted_at;
