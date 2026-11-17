-- invoice.record_arrears: record old arrears (owner, platform super_admin, and
-- an "admin" role where one exists — none is seeded; same pattern as 000054).
-- invoice.isolir_flag: change an invoice's isolation flag (owner + super_admin only).
WITH arrears_permissions(permission_name, module, action, description) AS (
	VALUES
		('invoice.record_arrears', 'receivable', 'record_arrears', 'Record old arrears as back-dated invoices'),
		('invoice.isolir_flag', 'receivable', 'isolir_flag', 'Change whether an invoice may trigger Mikrotik isolation')
),
upserted_permissions AS (
	INSERT INTO permissions (permission_name, module, action, name, slug, description, created_at, updated_at)
	SELECT permission_name, module, action, permission_name, permission_name, description, now(), now()
	FROM arrears_permissions
	ON CONFLICT (slug)
	DO UPDATE SET permission_name = EXCLUDED.permission_name, module = EXCLUDED.module, action = EXCLUDED.action,
		name = EXCLUDED.name, slug = EXCLUDED.slug, description = EXCLUDED.description, updated_at = now()
	RETURNING id, permission_name
),
grants AS (
	SELECT r.id AS role_id, p.id AS permission_id
	FROM upserted_permissions p
	JOIN roles r ON r.slug IN ('organization_owner', 'super_admin')
		OR (p.permission_name = 'invoice.record_arrears' AND r.slug = 'admin')
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT role_id, permission_id, 'organization', now()
FROM grants
ON CONFLICT (role_id, permission_id)
DO UPDATE SET scope = EXCLUDED.scope, granted_at = EXCLUDED.granted_at;
