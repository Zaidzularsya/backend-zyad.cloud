WITH billing_permissions(permission_name, action, description, permission_scope) AS (
	VALUES
		('platform.billing.plan.read', 'plan_read', 'Read billing plan catalog', 'all'),
		('platform.billing.plan.manage', 'plan_manage', 'Manage billing plans and prices', 'all'),
		('platform.billing.feature.read', 'feature_read', 'Read billing feature catalog', 'all'),
		('platform.billing.feature.manage', 'feature_manage', 'Manage billing feature catalog', 'all'),
		('platform.billing.entitlement.read', 'entitlement_read', 'Read billing plan entitlements', 'all'),
		('platform.billing.entitlement.manage', 'entitlement_manage', 'Manage billing plan entitlements', 'all'),
		('platform.billing.subscription.read', 'subscription_read', 'Read organization subscriptions', 'all'),
		('platform.billing.subscription.manage', 'subscription_manage', 'Manage organization subscriptions', 'all'),
		('platform.billing.invoice.read', 'invoice_read', 'Read billing invoices', 'all'),
		('platform.billing.invoice.manage', 'invoice_manage', 'Manage billing invoices', 'all'),
		('platform.billing.payment.manage', 'payment_manage', 'Manage billing payments', 'all'),
		('organization.billing.read', 'organization_billing_read', 'Read current organization billing', 'organization'),
		('organization.billing.manage', 'organization_billing_manage', 'Manage current organization billing requests', 'organization')
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
		'billing',
		action,
		permission_name,
		permission_name,
		description,
		now(),
		now()
	FROM billing_permissions
	ON CONFLICT (slug)
	DO UPDATE SET
		permission_name = EXCLUDED.permission_name,
		module = EXCLUDED.module,
		action = EXCLUDED.action,
		name = EXCLUDED.name,
		description = EXCLUDED.description,
		updated_at = now()
	RETURNING id, permission_name
),
platform_roles AS (
	SELECT id
	FROM roles
	WHERE role_name = 'super_admin'
		OR slug = 'super_admin'
),
platform_permissions AS (
	SELECT id
	FROM upserted_permissions
	WHERE permission_name LIKE 'platform.billing.%'
),
tenant_roles AS (
	SELECT id
	FROM roles
	WHERE role_name IN ('super_admin', 'organization_owner', 'admin')
		OR slug IN ('super_admin', 'organization_owner', 'admin')
),
tenant_permissions AS (
	SELECT id
	FROM upserted_permissions
	WHERE permission_name LIKE 'organization.billing.%'
),
platform_role_permissions AS (
	INSERT INTO role_permissions (
		role_id,
		permission_id,
		scope,
		granted_at
	)
	SELECT
		platform_roles.id,
		platform_permissions.id,
		'all',
		now()
	FROM platform_roles
	CROSS JOIN platform_permissions
	ON CONFLICT (role_id, permission_id)
	DO UPDATE SET
		scope = EXCLUDED.scope,
		granted_at = EXCLUDED.granted_at
	RETURNING role_id
)
INSERT INTO role_permissions (
	role_id,
	permission_id,
	scope,
	granted_at
)
SELECT
	tenant_roles.id,
	tenant_permissions.id,
	'organization',
	now()
FROM tenant_roles
CROSS JOIN tenant_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;
