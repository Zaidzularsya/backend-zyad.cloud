WITH organization_self_permissions(permission_name) AS (
	VALUES
		('organization.read'),
		('organization.update'),
		('organization.member.read'),
		('organization.member.manage')
),
deleted_role_permissions AS (
	DELETE FROM role_permissions
	WHERE permission_id IN (
		SELECT permission.id
		FROM permissions permission
		JOIN organization_self_permissions target
			ON target.permission_name = permission.permission_name
	)
	RETURNING permission_id
)
DELETE FROM permissions
WHERE permission_name IN (
	SELECT permission_name
	FROM organization_self_permissions
);
