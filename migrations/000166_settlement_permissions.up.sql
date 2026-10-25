-- Ledger penyelesaian: settlement.manage (super admin mencatat setoran/penyesuaian) dan
-- settlement.read (owner organisasi melihat saldo dan riwayat miliknya sendiri).
WITH new_permission AS (
	INSERT INTO permissions (
		permission_name, module, action, name, slug, description, created_at, updated_at
	)
	VALUES (
		'settlement.manage', 'settlement', 'manage', 'settlement.manage',
		'settlement.manage', 'Record manual settlements and adjustments in the platform settlement ledger', now(), now()
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
target_roles AS (
	SELECT id
	FROM roles
	WHERE role_name IN ('super_admin')
		OR slug IN ('super_admin')
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT target_roles.id, new_permission.id, 'organization', now()
FROM target_roles
CROSS JOIN new_permission
ON CONFLICT (role_id, permission_id)
DO UPDATE SET scope = EXCLUDED.scope, granted_at = EXCLUDED.granted_at;

WITH new_permission AS (
	INSERT INTO permissions (
		permission_name, module, action, name, slug, description, created_at, updated_at
	)
	VALUES (
		'settlement.read', 'settlement', 'read', 'settlement.read',
		'settlement.read', 'View the own organization settlement balance and entries', now(), now()
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
target_roles AS (
	SELECT id
	FROM roles
	WHERE role_name IN ('organization_owner')
		OR slug IN ('organization_owner')
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT target_roles.id, new_permission.id, 'organization', now()
FROM target_roles
CROSS JOIN new_permission
ON CONFLICT (role_id, permission_id)
DO UPDATE SET scope = EXCLUDED.scope, granted_at = EXCLUDED.granted_at;
