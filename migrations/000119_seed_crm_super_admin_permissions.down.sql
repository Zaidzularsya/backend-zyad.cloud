DELETE FROM role_permissions
USING permissions, roles
WHERE role_permissions.permission_id = permissions.id
	AND role_permissions.role_id = roles.id
	AND permissions.module = 'crm'
	AND (roles.role_name = 'super_admin' OR roles.slug = 'super_admin');
