CREATE TABLE IF NOT EXISTS crm_integrations (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	provider varchar(50) NOT NULL,
	name varchar(200) NOT NULL,
	config jsonb NOT NULL DEFAULT '{}'::jsonb,
	secret_encrypted text,
	is_active boolean NOT NULL DEFAULT true,
	connected_at timestamp without time zone,
	last_synced_at timestamp without time zone,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT crm_integrations_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT crm_integrations_name_not_blank_check CHECK (
		char_length(btrim(name)) > 0
	),
	CONSTRAINT crm_integrations_provider_check CHECK (
		provider IN ('webhook', 'whatsapp', 'email', 'zapier')
	),
	CONSTRAINT crm_integrations_config_object_check CHECK (
		jsonb_typeof(config) = 'object'
	),
	CONSTRAINT fk_crm_integrations_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_integrations_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_integrations_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_crm_integrations_organization_provider
	ON crm_integrations(organization_id, provider)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_integrations_deleted_at
	ON crm_integrations(deleted_at);

SELECT apply_organization_rls('crm_integrations'::regclass);
