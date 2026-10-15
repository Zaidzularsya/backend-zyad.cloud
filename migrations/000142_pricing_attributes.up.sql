ALTER TABLE catalog_products
	ADD COLUMN IF NOT EXISTS charge_type varchar(20) NOT NULL DEFAULT 'one_time',
	ADD COLUMN IF NOT EXISTS billing_frequency varchar(20) NULL,
	ADD COLUMN IF NOT EXISTS payment_timing varchar(20) NOT NULL DEFAULT 'prepaid';
ALTER TABLE catalog_products
	ADD CONSTRAINT catalog_products_charge_type_check CHECK (charge_type IN ('one_time', 'recurring')),
	ADD CONSTRAINT catalog_products_billing_frequency_check CHECK (billing_frequency IS NULL OR billing_frequency IN ('daily','weekly','monthly','quarterly','semiannual','annual')),
	ADD CONSTRAINT catalog_products_frequency_required CHECK ((charge_type = 'recurring') = (billing_frequency IS NOT NULL)),
	ADD CONSTRAINT catalog_products_payment_timing_check CHECK (payment_timing IN ('prepaid', 'postpaid'));

ALTER TABLE crm_quotation_items
	ADD COLUMN IF NOT EXISTS charge_type varchar(20) NOT NULL DEFAULT 'one_time',
	ADD COLUMN IF NOT EXISTS billing_frequency varchar(20) NULL,
	ADD COLUMN IF NOT EXISTS payment_timing varchar(20) NOT NULL DEFAULT 'prepaid';
ALTER TABLE crm_quotation_items
	ADD CONSTRAINT crm_quotation_items_charge_type_check CHECK (charge_type IN ('one_time', 'recurring')),
	ADD CONSTRAINT crm_quotation_items_billing_frequency_check CHECK (billing_frequency IS NULL OR billing_frequency IN ('daily','weekly','monthly','quarterly','semiannual','annual')),
	ADD CONSTRAINT crm_quotation_items_frequency_required CHECK ((charge_type = 'recurring') = (billing_frequency IS NOT NULL)),
	ADD CONSTRAINT crm_quotation_items_payment_timing_check CHECK (payment_timing IN ('prepaid', 'postpaid'));

ALTER TABLE crm_quotations
	ADD COLUMN IF NOT EXISTS one_time_total numeric(18, 2) NOT NULL DEFAULT 0,
	ADD COLUMN IF NOT EXISTS first_invoice_total numeric(18, 2) NOT NULL DEFAULT 0,
	ADD COLUMN IF NOT EXISTS recurring_totals jsonb NOT NULL DEFAULT '{}'::jsonb;
-- Backfill: semua baris lama = one_time + prepaid.
UPDATE crm_quotations SET one_time_total = grand_total, first_invoice_total = grand_total;
