-- R4-S3: entitlement workspace berasal dari contract. Fitur produk disalin
-- ke item SO & item contract; organization_entitlements mendapat source
-- 'contract' dan 'default'; permission company.link_workspace.
ALTER TABLE crm_sales_order_items
	ADD COLUMN IF NOT EXISTS features jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE receivable_contract_items
	ADD COLUMN IF NOT EXISTS features jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE organization_entitlements
	DROP CONSTRAINT IF EXISTS organization_entitlements_source_check;
ALTER TABLE organization_entitlements
	ADD CONSTRAINT organization_entitlements_source_check CHECK (
		source IN ('plan', 'addon', 'trial', 'platform_override', 'contract', 'default')
	);

WITH new_permission AS (
	INSERT INTO permissions (
		permission_name, module, action, name, slug, description, created_at, updated_at
	)
	VALUES (
		'company.link_workspace', 'crm', 'link_workspace', 'company.link_workspace',
		'company.link_workspace', 'Link a CRM company to a customer workspace', now(), now()
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
