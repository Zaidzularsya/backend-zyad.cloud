DROP INDEX IF EXISTS idx_crm_leads_converted_deal;
ALTER TABLE crm_deals
	DROP COLUMN IF EXISTS decision_maker,
	DROP COLUMN IF EXISTS description;
