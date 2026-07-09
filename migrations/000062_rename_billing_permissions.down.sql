WITH renames(new_permission_name, old_permission_name, old_module, old_name, old_slug) AS (
	VALUES
		('platform.product.plan.read', 'platform.billing.plan.read', 'billing', 'platform.billing.plan.read', 'platform.billing.plan.read'),
		('platform.product.plan.manage', 'platform.billing.plan.manage', 'billing', 'platform.billing.plan.manage', 'platform.billing.plan.manage'),
		('platform.product.plan_price.read', 'platform.billing.plan_price.read', 'billing', 'platform.billing.plan_price.read', 'platform.billing.plan_price.read'),
		('platform.product.plan_price.manage', 'platform.billing.plan_price.manage', 'billing', 'platform.billing.plan_price.manage', 'platform.billing.plan_price.manage'),
		('platform.product.feature.read', 'platform.billing.feature.read', 'billing', 'platform.billing.feature.read', 'platform.billing.feature.read'),
		('platform.product.feature.manage', 'platform.billing.feature.manage', 'billing', 'platform.billing.feature.manage', 'platform.billing.feature.manage'),
		('platform.product.entitlement.read', 'platform.billing.entitlement.read', 'billing', 'platform.billing.entitlement.read', 'platform.billing.entitlement.read'),
		('platform.product.entitlement.manage', 'platform.billing.entitlement.manage', 'billing', 'platform.billing.entitlement.manage', 'platform.billing.entitlement.manage'),
		('platform.subscription.read', 'platform.billing.subscription.read', 'billing', 'platform.billing.subscription.read', 'platform.billing.subscription.read'),
		('platform.subscription.manage', 'platform.billing.subscription.manage', 'billing', 'platform.billing.subscription.manage', 'platform.billing.subscription.manage')
)
UPDATE permissions
SET
	permission_name = renames.old_permission_name,
	module = renames.old_module,
	name = renames.old_name,
	slug = renames.old_slug,
	updated_at = now()
FROM renames
WHERE permissions.permission_name = renames.new_permission_name;
