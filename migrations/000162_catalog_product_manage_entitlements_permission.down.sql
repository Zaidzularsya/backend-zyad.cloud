DELETE FROM role_permissions
USING permissions
WHERE role_permissions.permission_id = permissions.id
	AND permissions.permission_name = 'catalog_product.manage_entitlements';
DELETE FROM permissions WHERE permission_name = 'catalog_product.manage_entitlements';
