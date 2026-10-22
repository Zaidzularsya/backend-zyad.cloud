DELETE FROM product_plan_entitlements
WHERE feature_id IN (
	SELECT id FROM product_features WHERE feature_key IN (
	'crm.max_leads',
	'crm.max_companies',
	'crm.max_deals',
	'crm.max_pipelines',
	'crm.max_quotations_per_month',
	'crm.max_sales_orders_per_month',
	'receivable.max_invoices_per_month'
	)
);

DELETE FROM product_features
WHERE feature_key IN (
	'crm.max_leads',
	'crm.max_companies',
	'crm.max_deals',
	'crm.max_pipelines',
	'crm.max_quotations_per_month',
	'crm.max_sales_orders_per_month',
	'receivable.max_invoices_per_month'
);
