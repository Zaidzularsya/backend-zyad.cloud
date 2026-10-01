ALTER TABLE crm_deals
	ADD COLUMN IF NOT EXISTS description text NULL,
	ADD COLUMN IF NOT EXISTS decision_maker varchar(150) NULL;

-- Deal Detail mencari lead asal lewat converted_deal_id.
CREATE INDEX IF NOT EXISTS idx_crm_leads_converted_deal
	ON crm_leads(organization_id, converted_deal_id)
	WHERE converted_deal_id IS NOT NULL;
