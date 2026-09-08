CREATE TABLE IF NOT EXISTS crm_activities (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	related_entity_type varchar(20) NOT NULL,
	related_entity_id uuid NOT NULL,
	type varchar(20) NOT NULL,
	subject varchar(200) NOT NULL,
	description text,
	due_at timestamp without time zone,
	completed_at timestamp without time zone,
	status varchar(20) NOT NULL DEFAULT 'pending',
	assignee_user_id uuid,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT crm_activities_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT crm_activities_subject_not_blank_check CHECK (
		char_length(btrim(subject)) > 0
	),
	CONSTRAINT crm_activities_related_entity_type_check CHECK (
		related_entity_type IN ('lead', 'contact', 'company', 'deal')
	),
	CONSTRAINT crm_activities_type_check CHECK (
		type IN ('call', 'email', 'meeting', 'task', 'note')
	),
	CONSTRAINT crm_activities_status_check CHECK (
		status IN ('pending', 'completed', 'cancelled')
	),
	CONSTRAINT crm_activities_completed_consistency_check CHECK (
		(status = 'completed' AND completed_at IS NOT NULL)
		OR (status <> 'completed' AND completed_at IS NULL)
	),
	CONSTRAINT fk_crm_activities_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_activities_assignee_user_id
		FOREIGN KEY (assignee_user_id) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_activities_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_activities_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

-- related_entity_id is intentionally NOT a foreign key: it points at one of
-- four different tables (crm_leads/crm_contacts/crm_companies/crm_deals)
-- depending on related_entity_type, which Postgres cannot express as a
-- single FK constraint. Existence is verified in the service layer
-- (ActivityService.validateRelatedEntity) instead — see
-- docs/reference-crm.md "Data Model" for the documented trade-off.

CREATE INDEX IF NOT EXISTS idx_crm_activities_organization_related
	ON crm_activities(organization_id, related_entity_type, related_entity_id)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_activities_organization_assignee_due
	ON crm_activities(organization_id, assignee_user_id, due_at)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_activities_organization_status
	ON crm_activities(organization_id, status)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_activities_deleted_at
	ON crm_activities(deleted_at);

SELECT apply_organization_rls('crm_activities'::regclass);
