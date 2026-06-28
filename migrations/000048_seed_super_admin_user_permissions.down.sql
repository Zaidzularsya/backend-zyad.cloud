WITH removable_permissions(permission_name) AS (
	VALUES
		('user.read'),
		('user.create'),
		('user.update'),
		('user.delete'),
		('user.restore'),
		('user.update_status'),
		('user.session.read'),
		('user.session.revoke'),
		('role.read'),
		('role.create'),
		('role.update'),
		('role.delete'),
		('role.assign'),
		('permission.read'),
		('permission.manage'),
		('audit.read'),
		('organization.user.read'),
		('organization.user.manage')
)
DELETE FROM role_permissions role_permission
USING roles role, permissions permission, removable_permissions removable
WHERE role_permission.role_id = role.id
	AND role_permission.permission_id = permission.id
	AND permission.permission_name = removable.permission_name
	AND (role.slug = 'super_admin' OR role.role_name = 'super_admin');
