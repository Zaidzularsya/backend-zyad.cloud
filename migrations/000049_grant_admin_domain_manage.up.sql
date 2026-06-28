WITH permission_target AS (
	SELECT id
	FROM permissions
	WHERE permission_name = 'organization.domain.manage'
		OR slug = 'organization.domain.manage'
	LIMIT 1
),
target_roles AS (
	SELECT id
	FROM roles
	WHERE role_name = 'admin'
		OR slug = 'admin'
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT target_roles.id, permission_target.id, 'all', now()
FROM target_roles
CROSS JOIN permission_target
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = now();
