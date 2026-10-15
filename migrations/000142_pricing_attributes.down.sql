ALTER TABLE crm_quotations
	DROP COLUMN IF EXISTS recurring_totals,
	DROP COLUMN IF EXISTS first_invoice_total,
	DROP COLUMN IF EXISTS one_time_total;

ALTER TABLE crm_quotation_items
	DROP CONSTRAINT IF EXISTS crm_quotation_items_payment_timing_check,
	DROP CONSTRAINT IF EXISTS crm_quotation_items_frequency_required,
	DROP CONSTRAINT IF EXISTS crm_quotation_items_billing_frequency_check,
	DROP CONSTRAINT IF EXISTS crm_quotation_items_charge_type_check;
ALTER TABLE crm_quotation_items
	DROP COLUMN IF EXISTS payment_timing,
	DROP COLUMN IF EXISTS billing_frequency,
	DROP COLUMN IF EXISTS charge_type;

ALTER TABLE catalog_products
	DROP CONSTRAINT IF EXISTS catalog_products_payment_timing_check,
	DROP CONSTRAINT IF EXISTS catalog_products_frequency_required,
	DROP CONSTRAINT IF EXISTS catalog_products_billing_frequency_check,
	DROP CONSTRAINT IF EXISTS catalog_products_charge_type_check;
ALTER TABLE catalog_products
	DROP COLUMN IF EXISTS payment_timing,
	DROP COLUMN IF EXISTS billing_frequency,
	DROP COLUMN IF EXISTS charge_type;
