CREATE TABLE IF NOT EXISTS crm_companies (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	name varchar(200) NOT NULL,
	industry varchar(120),
	website varchar(255),
	phone varchar(50),
	email varchar(255),
	address jsonb NOT NULL DEFAULT '{}'::jsonb,
	size_range varchar(50),
	notes text,
	tags text[] NOT NULL DEFAULT '{}',
	owner_user_id uuid,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT crm_companies_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT crm_companies_name_not_blank_check CHECK (
		char_length(btrim(name)) > 0
	),
	CONSTRAINT crm_companies_address_object_check CHECK (
		jsonb_typeof(address) = 'object'
	),
	CONSTRAINT fk_crm_companies_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_companies_owner_user_id
		FOREIGN KEY (owner_user_id) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_companies_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_companies_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_crm_companies_organization_name_active_unique
	ON crm_companies(organization_id, lower(name))
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_companies_organization_owner
	ON crm_companies(organization_id, owner_user_id)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_companies_deleted_at
	ON crm_companies(deleted_at);

SELECT apply_organization_rls('crm_companies'::regclass);
