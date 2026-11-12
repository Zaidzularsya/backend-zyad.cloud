-- Onboarding an existing customer (mikrotik secret) confirms its sales order
-- without an initial invoice; the contract then starts billing at the period
-- the receivable FirstPeriodIndex rule picks. Default false = previous flow.
ALTER TABLE crm_sales_orders ADD COLUMN IF NOT EXISTS skip_initial_invoice boolean NOT NULL DEFAULT false;
