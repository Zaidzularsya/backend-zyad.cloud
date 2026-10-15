ALTER TABLE crm_quotation_items
	DROP CONSTRAINT IF EXISTS fk_crm_quotation_items_product,
	DROP CONSTRAINT IF EXISTS crm_quotation_items_tax_percent_check,
	DROP COLUMN IF EXISTS tax_amount,
	DROP COLUMN IF EXISTS tax_percent,
	DROP COLUMN IF EXISTS unit,
	DROP COLUMN IF EXISTS sku,
	DROP COLUMN IF EXISTS product_id;
DROP INDEX IF EXISTS idx_crm_quotations_org_deal;
UPDATE crm_quotations SET status = 'rejected' WHERE status = 'superseded';
ALTER TABLE crm_quotations DROP CONSTRAINT IF EXISTS crm_quotations_status_check;
ALTER TABLE crm_quotations ADD CONSTRAINT crm_quotations_status_check CHECK (
	status IN ('draft', 'sent', 'approved', 'rejected', 'expired')
);
ALTER TABLE crm_quotations
	DROP CONSTRAINT IF EXISTS fk_crm_quotations_revision_of,
	DROP COLUMN IF EXISTS pdf_generated_at,
	DROP COLUMN IF EXISTS pdf_asset_id,
	DROP COLUMN IF EXISTS revision_no,
	DROP COLUMN IF EXISTS revision_of_id;
