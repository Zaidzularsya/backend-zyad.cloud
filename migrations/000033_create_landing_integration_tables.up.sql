CREATE TABLE IF NOT EXISTS landing_lead_integrations (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	name varchar(200) NOT NULL,
	type varchar(50) NOT NULL,
	credentials jsonb NOT NULL DEFAULT '{}'::jsonb,
	event_filters jsonb NOT NULL DEFAULT '[]'::jsonb,
	is_active boolean NOT NULL DEFAULT true,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT landing_lead_integrations_type_check CHECK (
		type IN ('notification', 'webhook', 'crm', 'telegram', 'google_sheets')
	),
	CONSTRAINT landing_lead_integrations_credentials_object_check CHECK (jsonb_typeof(credentials) = 'object'),
	CONSTRAINT landing_lead_integrations_event_filters_array_check CHECK (jsonb_typeof(event_filters) = 'array'),
	CONSTRAINT landing_lead_integrations_name_not_blank_check CHECK (char_length(btrim(name)) > 0),
	CONSTRAINT landing_lead_integrations_org_id_unique UNIQUE (organization_id, id),
	CONSTRAINT fk_landing_lead_integrations_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_lead_integrations_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_landing_lead_integrations_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_landing_lead_integrations_deleted_at
	ON landing_lead_integrations(deleted_at);

CREATE INDEX IF NOT EXISTS idx_landing_lead_integrations_org_active
	ON landing_lead_integrations(organization_id, is_active)
	WHERE deleted_at IS NULL;


CREATE TABLE IF NOT EXISTS landing_lead_delivery_logs (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	integration_id uuid NOT NULL,
	submission_id uuid NOT NULL,
	status varchar(30) NOT NULL DEFAULT 'pending',
	response_payload text,
	error_message text,
	attempts integer NOT NULL DEFAULT 0,
	next_retry_at timestamp without time zone,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT landing_lead_delivery_logs_status_check CHECK (status IN ('pending', 'success', 'failed')),
	CONSTRAINT landing_lead_delivery_logs_attempts_check CHECK (attempts >= 0),
	CONSTRAINT fk_landing_lead_delivery_logs_integration
		FOREIGN KEY (organization_id, integration_id)
		REFERENCES landing_lead_integrations(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_lead_delivery_logs_submission
		FOREIGN KEY (organization_id, submission_id)
		REFERENCES landing_submissions(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_lead_delivery_logs_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_landing_lead_delivery_logs_claim
	ON landing_lead_delivery_logs(status, next_retry_at)
	WHERE status = 'pending';

SELECT apply_organization_rls('landing_lead_integrations'::regclass);
-- landing_lead_delivery_logs does not have RLS so background workers can claim logs globally
