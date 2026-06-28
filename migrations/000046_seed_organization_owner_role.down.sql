DELETE FROM role_permissions
WHERE role_id IN (
	SELECT id
	FROM roles
	WHERE slug = 'organization_owner'
		AND role_name = 'organization_owner'
);

DELETE FROM roles
WHERE slug = 'organization_owner'
	AND role_name = 'organization_owner';
