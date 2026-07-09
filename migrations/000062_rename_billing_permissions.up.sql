-- Rename permission slugs to match the product/subscription/billing domain split.
-- Uses UPDATE (not delete+insert) so existing role_permissions assignments (by permission_id)
-- remain intact.

WITH renames(old_permission_name, new_permission_name, new_module, new_name, new_slug) AS (
	VALUES
		('platform.billing.plan.read', 'platform.product.plan.read', 'product', 'platform.product.plan.read', 'platform.product.plan.read'),
		('platform.billing.plan.manage', 'platform.product.plan.manage', 'product', 'platform.product.plan.manage', 'platform.product.plan.manage'),
		('platform.billing.plan_price.read', 'platform.product.plan_price.read', 'product', 'platform.product.plan_price.read', 'platform.product.plan_price.read'),
		('platform.billing.plan_price.manage', 'platform.product.plan_price.manage', 'product', 'platform.product.plan_price.manage', 'platform.product.plan_price.manage'),
		('platform.billing.feature.read', 'platform.product.feature.read', 'product', 'platform.product.feature.read', 'platform.product.feature.read'),
		('platform.billing.feature.manage', 'platform.product.feature.manage', 'product', 'platform.product.feature.manage', 'platform.product.feature.manage'),
		('platform.billing.entitlement.read', 'platform.product.entitlement.read', 'product', 'platform.product.entitlement.read', 'platform.product.entitlement.read'),
		('platform.billing.entitlement.manage', 'platform.product.entitlement.manage', 'product', 'platform.product.entitlement.manage', 'platform.product.entitlement.manage'),
		('platform.billing.subscription.read', 'platform.subscription.read', 'subscription', 'platform.subscription.read', 'platform.subscription.read'),
		('platform.billing.subscription.manage', 'platform.subscription.manage', 'subscription', 'platform.subscription.manage', 'platform.subscription.manage')
)
UPDATE permissions
SET
	permission_name = renames.new_permission_name,
	module = renames.new_module,
	name = renames.new_name,
	slug = renames.new_slug,
	updated_at = now()
FROM renames
WHERE permissions.permission_name = renames.old_permission_name;
