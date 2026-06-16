WITH platform_organization_permissions(permission_name) AS (
	VALUES
		('platform.organization.read'),
		('platform.organization.manage'),
		('platform.organization.suspend'),
		('platform.organization.provision')
),
deleted_role_permissions AS (
	DELETE FROM role_permissions
	WHERE permission_id IN (
		SELECT permission.id
		FROM permissions permission
		JOIN platform_organization_permissions target
			ON target.permission_name = permission.permission_name
	)
	RETURNING permission_id
)
DELETE FROM permissions
WHERE permission_name IN (
	SELECT permission_name
	FROM platform_organization_permissions
);
