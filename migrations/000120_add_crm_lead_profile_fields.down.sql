ALTER TABLE crm_leads
	DROP CONSTRAINT IF EXISTS crm_leads_annual_revenue_check;

ALTER TABLE crm_leads
	DROP COLUMN IF EXISTS address,
	DROP COLUMN IF EXISTS annual_revenue,
	DROP COLUMN IF EXISTS job_title;
