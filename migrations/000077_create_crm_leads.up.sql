CREATE TABLE IF NOT EXISTS crm_leads (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	contact_name varchar(200) NOT NULL,
	company_name varchar(200),
	email varchar(255),
	phone varchar(50),
	source varchar(50),
	status varchar(20) NOT NULL DEFAULT 'new',
	score integer NOT NULL DEFAULT 0,
	owner_user_id uuid,
	notes text,
	converted_contact_id uuid,
	converted_company_id uuid,
	converted_deal_id uuid,
	converted_at timestamp without time zone,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT crm_leads_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT crm_leads_contact_name_not_blank_check CHECK (
		char_length(btrim(contact_name)) > 0
	),
	CONSTRAINT crm_leads_status_check CHECK (
		status IN ('new', 'contacted', 'qualified', 'unqualified', 'converted')
	),
	CONSTRAINT crm_leads_score_check CHECK (
		score >= 0 AND score <= 100
	),
	CONSTRAINT crm_leads_converted_consistency_check CHECK (
		(status = 'converted' AND converted_at IS NOT NULL)
		OR (status <> 'converted' AND converted_at IS NULL
			AND converted_contact_id IS NULL
			AND converted_company_id IS NULL
			AND converted_deal_id IS NULL)
	),
	CONSTRAINT fk_crm_leads_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_leads_converted_contact
		FOREIGN KEY (organization_id, converted_contact_id)
		REFERENCES crm_contacts(organization_id, id)
		ON DELETE SET NULL,
	CONSTRAINT fk_crm_leads_converted_company
		FOREIGN KEY (organization_id, converted_company_id)
		REFERENCES crm_companies(organization_id, id)
		ON DELETE SET NULL,
	CONSTRAINT fk_crm_leads_owner_user_id
		FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_leads_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_leads_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

-- converted_deal_id intentionally left without a FK constraint here — the
-- crm_deals table does not exist yet (Fase 2). The FK is added by
-- migration 000079 once crm_deals is created.

CREATE INDEX IF NOT EXISTS idx_crm_leads_organization_status
	ON crm_leads(organization_id, status)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_leads_organization_owner
	ON crm_leads(organization_id, owner_user_id)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_leads_deleted_at
	ON crm_leads(deleted_at);

SELECT apply_organization_rls('crm_leads'::regclass);
