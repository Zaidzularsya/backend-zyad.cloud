CREATE TABLE IF NOT EXISTS crm_deals (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	pipeline_id uuid NOT NULL,
	stage_id uuid NOT NULL,
	company_id uuid,
	contact_id uuid,
	title varchar(200) NOT NULL,
	value numeric(18, 2) NOT NULL DEFAULT 0,
	currency char(3) NOT NULL DEFAULT 'IDR',
	expected_close_date date,
	status varchar(20) NOT NULL DEFAULT 'open',
	lost_reason text,
	owner_user_id uuid,
	discount_percent numeric(5, 2),
	discount_approved_by uuid,
	discount_approved_at timestamp without time zone,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT crm_deals_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT crm_deals_title_not_blank_check CHECK (
		char_length(btrim(title)) > 0
	),
	CONSTRAINT crm_deals_value_check CHECK (value >= 0),
	CONSTRAINT crm_deals_status_check CHECK (
		status IN ('open', 'won', 'lost')
	),
	CONSTRAINT crm_deals_discount_percent_check CHECK (
		discount_percent IS NULL OR (discount_percent >= 0 AND discount_percent <= 100)
	),
	CONSTRAINT crm_deals_lost_reason_consistency_check CHECK (
		(status = 'lost') OR (lost_reason IS NULL)
	),
	CONSTRAINT fk_crm_deals_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_deals_pipeline
		FOREIGN KEY (organization_id, pipeline_id)
		REFERENCES crm_pipelines(organization_id, id)
		ON DELETE RESTRICT,
	CONSTRAINT fk_crm_deals_stage
		FOREIGN KEY (organization_id, stage_id)
		REFERENCES crm_pipeline_stages(organization_id, id)
		ON DELETE RESTRICT,
	CONSTRAINT fk_crm_deals_company
		FOREIGN KEY (organization_id, company_id)
		REFERENCES crm_companies(organization_id, id)
		ON DELETE SET NULL,
	CONSTRAINT fk_crm_deals_contact
		FOREIGN KEY (organization_id, contact_id)
		REFERENCES crm_contacts(organization_id, id)
		ON DELETE SET NULL,
	CONSTRAINT fk_crm_deals_owner_user_id
		FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_deals_discount_approved_by
		FOREIGN KEY (discount_approved_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_deals_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_deals_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_crm_deals_organization_pipeline_stage
	ON crm_deals(organization_id, pipeline_id, stage_id)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_deals_organization_status
	ON crm_deals(organization_id, status)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_deals_organization_owner
	ON crm_deals(organization_id, owner_user_id)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_deals_deleted_at
	ON crm_deals(deleted_at);

SELECT apply_organization_rls('crm_deals'::regclass);

-- crm_leads.converted_deal_id was left without a FK in migration 000077
-- because crm_deals didn't exist yet. Add it now (Fase 2).
ALTER TABLE crm_leads
	ADD CONSTRAINT fk_crm_leads_converted_deal
	FOREIGN KEY (organization_id, converted_deal_id)
	REFERENCES crm_deals(organization_id, id)
	ON DELETE SET NULL;
