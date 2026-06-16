DELETE FROM role_permissions
WHERE permission_id IN (
	SELECT id
	FROM permissions
	WHERE permission_name = 'organization.feature.read'
);

DELETE FROM permissions
WHERE permission_name = 'organization.feature.read';
