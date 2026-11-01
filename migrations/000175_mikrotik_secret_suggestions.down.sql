DELETE FROM role_permissions
USING roles, permissions
WHERE role_permissions.role_id = roles.id
	AND role_permissions.permission_id = permissions.id
	AND (roles.role_name = 'member' OR roles.slug = 'member')
	AND permissions.permission_name = 'mikrotik_task.read';

DROP TABLE IF EXISTS mikrotik_secret_suggestions;
