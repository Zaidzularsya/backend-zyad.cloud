-- Grants super_admin (platform) the same CRM permission set
-- organization_owner already has, now that platform-type organizations are
-- allowed into /app/crm (see internal/core/middleware/tenant.go
-- RequireCustomerOrPlatformTenant and internal/app/router.go). Migration
-- 000078 intentionally excluded super_admin entirely — this migration
-- reverses that exclusion now that the platform organization uses CRM for
-- its own sales/lead tracking.
--
-- Grants every existing permission with module = 'crm' rather than
-- hardcoding names, so this stays correct as CRM gains resources without
-- needing another seed migration.
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT
	super_admin_roles.id,
	permissions.id,
	'organization',
	now()
FROM permissions
CROSS JOIN (
	SELECT id
	FROM roles
	WHERE role_name = 'super_admin' OR slug = 'super_admin'
) AS super_admin_roles
WHERE permissions.module = 'crm'
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;
