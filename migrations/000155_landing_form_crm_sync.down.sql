ALTER TABLE landing_submissions
	DROP COLUMN IF EXISTS crm_sync_error,
	DROP COLUMN IF EXISTS crm_sync_status,
	DROP COLUMN IF EXISTS crm_lead_id;

ALTER TABLE landing_forms
	DROP COLUMN IF EXISTS lead_owner_user_id,
	DROP COLUMN IF EXISTS create_crm_lead;
