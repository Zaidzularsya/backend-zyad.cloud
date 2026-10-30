-- Permission router Mikrotik. Hanya organization_owner; member tidak mendapat grant.
WITH mikrotik_permissions(permission_name, module, action, description) AS (
	VALUES
		('mikrotik_router.read', 'mikrotik', 'read', 'Read Mikrotik routers'),
		('mikrotik_router.create', 'mikrotik', 'create', 'Register Mikrotik routers'),
		('mikrotik_router.update', 'mikrotik', 'update', 'Update Mikrotik routers'),
		('mikrotik_router.delete', 'mikrotik', 'delete', 'Delete Mikrotik routers'),
		('mikrotik_router.setup', 'mikrotik', 'setup', 'Run isolir setup on Mikrotik routers')
),
upserted_permissions AS (
	INSERT INTO permissions (permission_name, module, action, name, slug, description, created_at, updated_at)
	SELECT permission_name, module, action, permission_name, permission_name, description, now(), now()
	FROM mikrotik_permissions
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
