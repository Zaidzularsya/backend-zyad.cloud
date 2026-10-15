-- Link lead -> file di storage tenant (asset_objects). File-nya sendiri tetap
-- dimiliki modul asset (kuota storage.max_bytes, presign download), tabel ini
-- hanya mencatat file mana yang menempel ke lead mana.
CREATE TABLE IF NOT EXISTS crm_lead_attachments (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	lead_id uuid NOT NULL,
	asset_object_id uuid NOT NULL,
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT crm_lead_attachments_asset_object_unique UNIQUE (asset_object_id),
	CONSTRAINT fk_crm_lead_attachments_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_lead_attachments_lead
		FOREIGN KEY (organization_id, lead_id)
		REFERENCES crm_leads(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_crm_lead_attachments_asset_object
		FOREIGN KEY (asset_object_id) REFERENCES asset_objects(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_lead_attachments_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_crm_lead_attachments_org_lead
	ON crm_lead_attachments(organization_id, lead_id, created_at DESC);

SELECT apply_organization_rls('crm_lead_attachments'::regclass);
