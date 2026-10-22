-- Quota (integer) features for CRM and receivable. Plan entitlement values
-- are assigned separately; registering the keys alone changes no behavior.
INSERT INTO product_features (
	feature_key,
	module,
	name,
	description,
	value_type,
	unit,
	reset_strategy,
	is_active,
	created_at,
	updated_at
)
VALUES
	('crm.max_leads', 'crm', 'Maximum CRM leads', 'Maximum number of leads an organization may hold', 'integer', 'lead', 'never', true, now(), now()),
	('crm.max_companies', 'crm', 'Maximum CRM companies', 'Maximum number of companies an organization may hold', 'integer', 'company', 'never', true, now(), now()),
	('crm.max_deals', 'crm', 'Maximum CRM deals', 'Maximum number of deals an organization may hold', 'integer', 'deal', 'never', true, now(), now()),
	('crm.max_pipelines', 'crm', 'Maximum CRM pipelines', 'Maximum number of pipelines an organization may create', 'integer', 'pipeline', 'never', true, now(), now()),
	('crm.max_quotations_per_month', 'crm', 'Maximum quotations per month', 'Maximum number of quotations created per calendar month', 'integer', 'quotation', 'monthly', true, now(), now()),
	('crm.max_sales_orders_per_month', 'crm', 'Maximum sales orders per month', 'Maximum number of sales orders created per calendar month', 'integer', 'sales order', 'monthly', true, now(), now()),
	('receivable.max_invoices_per_month', 'receivable', 'Maximum invoices per month', 'Maximum number of invoices created per calendar month', 'integer', 'invoice', 'monthly', true, now(), now())
ON CONFLICT (feature_key)
DO NOTHING;
