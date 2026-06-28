DELETE FROM role_permissions role_permission
USING roles role, permissions permission
WHERE role_permission.role_id = role.id
	AND role_permission.permission_id = permission.id
	AND permission.permission_name = 'organization.domain.manage'
	AND (role.role_name = 'admin' OR role.slug = 'admin');
