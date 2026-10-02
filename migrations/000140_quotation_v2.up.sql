ALTER TABLE crm_quotations
	ADD COLUMN IF NOT EXISTS revision_of_id uuid NULL,
	ADD COLUMN IF NOT EXISTS revision_no integer NOT NULL DEFAULT 0,
	ADD COLUMN IF NOT EXISTS pdf_asset_id uuid NULL,
	ADD COLUMN IF NOT EXISTS pdf_generated_at timestamp without time zone NULL;

ALTER TABLE crm_quotations
	ADD CONSTRAINT fk_crm_quotations_revision_of
	FOREIGN KEY (organization_id, revision_of_id)
	REFERENCES crm_quotations(organization_id, id) ON DELETE SET NULL (revision_of_id);

ALTER TABLE crm_quotations DROP CONSTRAINT IF EXISTS crm_quotations_status_check;
ALTER TABLE crm_quotations ADD CONSTRAINT crm_quotations_status_check CHECK (
	status IN ('draft', 'sent', 'approved', 'rejected', 'expired', 'superseded')
);

CREATE INDEX IF NOT EXISTS idx_crm_quotations_org_deal
	ON crm_quotations(organization_id, deal_id) WHERE deleted_at IS NULL;

ALTER TABLE crm_quotation_items
	ADD COLUMN IF NOT EXISTS product_id uuid NULL,
	ADD COLUMN IF NOT EXISTS sku varchar(64) NULL,
	ADD COLUMN IF NOT EXISTS unit varchar(30) NULL,
	ADD COLUMN IF NOT EXISTS tax_percent numeric(5, 2) NOT NULL DEFAULT 0,
	ADD COLUMN IF NOT EXISTS tax_amount numeric(18, 2) NOT NULL DEFAULT 0;

ALTER TABLE crm_quotation_items
	ADD CONSTRAINT crm_quotation_items_tax_percent_check CHECK (tax_percent >= 0 AND tax_percent <= 100),
	ADD CONSTRAINT fk_crm_quotation_items_product
	FOREIGN KEY (organization_id, product_id)
	REFERENCES catalog_products(organization_id, id) ON DELETE SET NULL (product_id);
