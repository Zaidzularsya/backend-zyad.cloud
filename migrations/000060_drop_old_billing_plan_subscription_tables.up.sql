-- Repoint billing_invoices' subscription FK from the old billing_subscriptions table
-- to the new customer_subscriptions table before dropping the old table.
ALTER TABLE billing_invoices
	DROP CONSTRAINT IF EXISTS fk_billing_invoices_subscription_org;

ALTER TABLE billing_invoices
	ADD CONSTRAINT fk_billing_invoices_subscription_org
	FOREIGN KEY (subscription_id, organization_id)
	REFERENCES customer_subscriptions(id, organization_id)
	ON DELETE RESTRICT;

-- Drop old product/subscription tables in dependency order.
DROP TABLE IF EXISTS billing_plan_entitlements;
DROP TABLE IF EXISTS billing_subscription_events;
DROP TABLE IF EXISTS billing_subscriptions;
DROP TABLE IF EXISTS billing_features;
DROP TABLE IF EXISTS billing_plan_prices;
DROP TABLE IF EXISTS billing_plans;
