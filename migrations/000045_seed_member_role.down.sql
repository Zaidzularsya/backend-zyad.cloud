DELETE FROM roles
WHERE slug = 'member'
	AND role_name = 'member'
	AND NOT EXISTS (
		SELECT 1
		FROM user_roles
		WHERE user_roles.role_id = roles.id
	)
	AND NOT EXISTS (
		SELECT 1
		FROM role_permissions
		WHERE role_permissions.role_id = roles.id
	);
