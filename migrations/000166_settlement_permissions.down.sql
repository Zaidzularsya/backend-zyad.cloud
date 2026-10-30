DELETE FROM role_permissions
USING permissions
WHERE role_permissions.permission_id = permissions.id
	AND permissions.permission_name IN ('settlement.manage', 'settlement.read');
DELETE FROM permissions WHERE permission_name IN ('settlement.manage', 'settlement.read');
