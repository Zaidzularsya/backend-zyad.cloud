-- Permission impor & pemetaan secret, kebijakan pelanggan, penanda produk (Fase 2). Hanya organization_owner.
WITH mikrotik_secret_permissions(permission_name, module, action, description) AS (
	VALUES
		('mikrotik_secret.read', 'mikrotik', 'read', 'Read imported Mikrotik PPP secrets'),
		('mikrotik_secret.import', 'mikrotik', 'import', 'Import PPP secrets from Mikrotik routers'),
		('mikrotik_secret.pair', 'mikrotik', 'pair', 'Pair PPP secrets to customers'),
		('mikrotik_secret.unpair', 'mikrotik', 'unpair', 'Unpair PPP secrets from customers'),
		('mikrotik_policy.read', 'mikrotik', 'read', 'Read customer isolir policies'),
		('mikrotik_policy.update', 'mikrotik', 'update', 'Update customer isolir policies'),
		('mikrotik_product_trigger.read', 'mikrotik', 'read', 'Read PPPoE product triggers'),
		('mikrotik_product_trigger.update', 'mikrotik', 'update', 'Mark products as PPPoE service')
),
upserted_permissions AS (
	INSERT INTO permissions (permission_name, module, action, name, slug, description, created_at, updated_at)
	SELECT permission_name, module, action, permission_name, permission_name, description, now(), now()
	FROM mikrotik_secret_permissions
	ON CONFLICT (slug)
	DO UPDATE SET permission_name = EXCLUDED.permission_name, module = EXCLUDED.module, action = EXCLUDED.action,
		name = EXCLUDED.name, slug = EXCLUDED.slug, description = EXCLUDED.description, updated_at = now()
	RETURNING id
),
owner_roles AS (
	SELECT id
	FROM roles
	WHERE role_name IN ('organization_owner')
		OR slug IN ('organization_owner')
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT owner_roles.id, upserted_permissions.id, 'organization', now()
FROM owner_roles
CROSS JOIN upserted_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET scope = EXCLUDED.scope, granted_at = EXCLUDED.granted_at;
