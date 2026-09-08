CREATE TABLE IF NOT EXISTS crm_contacts (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	company_id uuid,
	first_name varchar(120) NOT NULL,
	last_name varchar(120),
	email varchar(255),
	phone varchar(50),
	job_title varchar(120),
	address jsonb NOT NULL DEFAULT '{}'::jsonb,
	tags text[] NOT NULL DEFAULT '{}',
	source varchar(50),
	owner_user_id uuid,
	is_customer boolean NOT NULL DEFAULT false,
	lifecycle_stage varchar(20) NOT NULL DEFAULT 'contact',
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT crm_contacts_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT crm_contacts_first_name_not_blank_check CHECK (
		char_length(btrim(first_name)) > 0
	),
	CONSTRAINT crm_contacts_address_object_check CHECK (
		jsonb_typeof(address) = 'object'
	),
	CONSTRAINT crm_contacts_lifecycle_stage_check CHECK (
		lifecycle_stage IN ('lead', 'contact', 'customer', 'churned')
	),
	CONSTRAINT fk_crm_contacts_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_contacts_company
		FOREIGN KEY (organization_id, company_id)
		REFERENCES crm_companies(organization_id, id)
		ON DELETE SET NULL,
	CONSTRAINT fk_crm_contacts_owner_user_id
		FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_contacts_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_contacts_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_crm_contacts_organization_email
	ON crm_contacts(organization_id, email)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_contacts_organization_company
	ON crm_contacts(organization_id, company_id)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_contacts_organization_lifecycle
	ON crm_contacts(organization_id, lifecycle_stage)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_contacts_deleted_at
	ON crm_contacts(deleted_at);

SELECT apply_organization_rls('crm_contacts'::regclass);
