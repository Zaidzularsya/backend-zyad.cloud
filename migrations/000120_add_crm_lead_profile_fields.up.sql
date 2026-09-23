-- Field profil tambahan untuk panel detail lead (Job Title, Annual revenue,
-- Address info). address mengikuti bentuk crm_contacts.address (jsonb bebas:
-- street/city/state/postal_code/country) supaya bisa langsung dibawa saat
-- convert ke contact.
ALTER TABLE crm_leads
	ADD COLUMN IF NOT EXISTS job_title varchar(150),
	ADD COLUMN IF NOT EXISTS annual_revenue numeric(18, 2),
	ADD COLUMN IF NOT EXISTS address jsonb NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE crm_leads
	DROP CONSTRAINT IF EXISTS crm_leads_annual_revenue_check;
ALTER TABLE crm_leads
	ADD CONSTRAINT crm_leads_annual_revenue_check CHECK (
		annual_revenue IS NULL OR annual_revenue >= 0
	);
