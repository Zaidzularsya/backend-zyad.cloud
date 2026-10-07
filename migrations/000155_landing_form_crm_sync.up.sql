-- R5-S4: form landing -> lead CRM. Kolom aditif; baris lama mendapat default aman
-- (form lama ikut membuat lead; submission lama berstatus 'skipped').
ALTER TABLE landing_forms
	ADD COLUMN create_crm_lead boolean NOT NULL DEFAULT true,
	ADD COLUMN lead_owner_user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL;

ALTER TABLE landing_submissions
	ADD COLUMN crm_lead_id uuid NULL,
	ADD COLUMN crm_sync_status varchar(20) NOT NULL DEFAULT 'skipped'
		CHECK (crm_sync_status IN ('skipped', 'created', 'merged', 'failed')),
	ADD COLUMN crm_sync_error text NULL;
