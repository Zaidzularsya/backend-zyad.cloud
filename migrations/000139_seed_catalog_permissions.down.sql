DELETE FROM role_permissions
USING permissions
WHERE role_permissions.permission_id = permissions.id
	AND permissions.permission_name LIKE 'catalog_product.%';
DELETE FROM permissions WHERE permission_name LIKE 'catalog_product.%';
