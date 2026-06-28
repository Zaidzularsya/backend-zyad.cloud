WITH required_permissions(permission_name, description) AS (
	VALUES
		('user.read', 'Read user data'),
		('user.create', 'Create user'),
		('user.update', 'Update user'),
		('user.delete', 'Delete user'),
		('user.restore', 'Restore deleted user'),
		('user.update_status', 'Update user status'),
		('user.session.read', 'Read user sessions'),
		('user.session.revoke', 'Revoke user sessions'),
		('role.read', 'Read role data'),
		('role.create', 'Create role'),
		('role.update', 'Update role'),
		('role.delete', 'Delete role'),
		('role.assign', 'Assign roles to users'),
		('permission.read', 'Read permission data'),
		('permission.manage', 'Manage permission data'),
		('audit.read', 'Read audit logs'),
		('organization.user.read', 'Read organization users'),
		('organization.user.manage', 'Manage organization users')
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
		split_part(permission_name, '.', 1),
		split_part(permission_name, '.', 2),
		permission_name,
		permission_name,
		description,
		now(),
		now()
	FROM required_permissions
	ON CONFLICT (permission_name)
	DO UPDATE SET
		module = EXCLUDED.module,
		action = EXCLUDED.action,
		name = EXCLUDED.name,
		slug = EXCLUDED.slug,
		description = EXCLUDED.description,
		updated_at = now()
	RETURNING id
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT role.id, permission.id, 'all', now()
FROM roles role
CROSS JOIN upserted_permissions permission
WHERE role.slug = 'super_admin'
	OR role.role_name = 'super_admin'
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = now();
