-- Link contact -> file di storage tenant (asset_objects). File-nya sendiri tetap
-- dimiliki modul asset (kuota storage.max_bytes, presign download), tabel ini
-- hanya mencatat file mana yang menempel ke contact mana.
CREATE TABLE IF NOT EXISTS crm_contact_attachments (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	contact_id uuid NOT NULL,
	asset_object_id uuid NOT NULL,
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT crm_contact_attachments_asset_object_unique UNIQUE (asset_object_id),
	CONSTRAINT fk_crm_contact_attachments_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_contact_attachments_contact
		FOREIGN KEY (organization_id, contact_id)
		REFERENCES crm_contacts(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_crm_contact_attachments_asset_object
		FOREIGN KEY (asset_object_id) REFERENCES asset_objects(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_contact_attachments_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_crm_contact_attachments_org_contact
	ON crm_contact_attachments(organization_id, contact_id, created_at DESC);

SELECT apply_organization_rls('crm_contact_attachments'::regclass);
