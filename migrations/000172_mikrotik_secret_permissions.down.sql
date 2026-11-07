DELETE FROM role_permissions
USING permissions
WHERE role_permissions.permission_id = permissions.id
	AND permissions.permission_name ~ '^mikrotik_(secret|policy|product_trigger)\.';
DELETE FROM permissions WHERE permission_name ~ '^mikrotik_(secret|policy|product_trigger)\.';
