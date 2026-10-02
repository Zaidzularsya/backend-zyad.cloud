-- Permissions for the tenant product catalog (catalog module). organization_owner
-- and super_admin get full access; member gets read only (quotation item picker).
WITH catalog_permissions(permission_name, module, action, description) AS (
	VALUES
		('catalog_product.read', 'catalog', 'read', 'Read tenant product catalog'),
		('catalog_product.create', 'catalog', 'create', 'Create catalog products and categories'),
		('catalog_product.update', 'catalog', 'update', 'Update catalog products and categories'),
		('catalog_product.delete', 'catalog', 'delete', 'Delete catalog products and categories')
),
upserted_permissions AS (
	INSERT INTO permissions (
		permission_name,
		module,
		action,
		name,
		slug,
		description,
		created_at,
		updated_at
	)
	SELECT
		permission_name,
		module,
		action,
		permission_name,
		permission_name,
		description,
		now(),
		now()
	FROM catalog_permissions
	ON CONFLICT (slug)
	DO UPDATE SET
		permission_name = EXCLUDED.permission_name,
		module = EXCLUDED.module,
		action = EXCLUDED.action,
		name = EXCLUDED.name,
		slug = EXCLUDED.slug,
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
SELECT
	full_access_roles.id,
	upserted_permissions.id,
	'organization',
	now()
FROM full_access_roles
CROSS JOIN upserted_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;

WITH member_permission_names(permission_name) AS (
	VALUES
		('catalog_product.read')
),
member_roles AS (
	SELECT id
	FROM roles
	WHERE role_name = 'member' OR slug = 'member'
),
target_permissions AS (
	SELECT id
	FROM permissions
	WHERE permission_name IN (SELECT permission_name FROM member_permission_names)
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT
	member_roles.id,
	target_permissions.id,
	'organization',
	now()
FROM member_roles
CROSS JOIN target_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;
