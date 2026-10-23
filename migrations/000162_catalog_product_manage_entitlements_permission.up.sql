-- Tenant products: izin melampirkan fitur (entitlement) pada produk tenant.
WITH new_permission AS (
	INSERT INTO permissions (
		permission_name, module, action, name, slug, description, created_at, updated_at
	)
	VALUES (
		'catalog_product.manage_entitlements', 'catalog', 'manage_entitlements', 'catalog_product.manage_entitlements',
		'catalog_product.manage_entitlements', 'Attach offerable features to tenant catalog products', now(), now()
	)
	ON CONFLICT (slug)
	DO UPDATE SET
		permission_name = EXCLUDED.permission_name,
		module = EXCLUDED.module,
		action = EXCLUDED.action,
		name = EXCLUDED.name,
		description = EXCLUDED.description,
		updated_at = now()
	RETURNING id
),
full_access_roles AS (
	SELECT id
	FROM roles
	WHERE role_name IN ('organization_owner', 'super_admin')
		OR slug IN ('organization_owner', 'super_admin')
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT full_access_roles.id, new_permission.id, 'organization', now()
FROM full_access_roles
CROSS JOIN new_permission
ON CONFLICT (role_id, permission_id)
DO UPDATE SET scope = EXCLUDED.scope, granted_at = EXCLUDED.granted_at;
