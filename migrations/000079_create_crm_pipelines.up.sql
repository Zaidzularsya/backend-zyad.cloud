CREATE TABLE IF NOT EXISTS crm_pipelines (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	name varchar(200) NOT NULL,
	is_default boolean NOT NULL DEFAULT false,
	archived_at timestamp without time zone,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT crm_pipelines_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT crm_pipelines_name_not_blank_check CHECK (
		char_length(btrim(name)) > 0
	),
	CONSTRAINT fk_crm_pipelines_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_pipelines_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_pipelines_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_crm_pipelines_organization_default_unique
	ON crm_pipelines(organization_id)
	WHERE is_default = true AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_pipelines_deleted_at
	ON crm_pipelines(deleted_at);

SELECT apply_organization_rls('crm_pipelines'::regclass);

CREATE TABLE IF NOT EXISTS crm_pipeline_stages (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	pipeline_id uuid NOT NULL,
	name varchar(200) NOT NULL,
	position integer NOT NULL,
	probability numeric(5, 2) NOT NULL DEFAULT 0,
	is_won boolean NOT NULL DEFAULT false,
	is_lost boolean NOT NULL DEFAULT false,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT crm_pipeline_stages_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT crm_pipeline_stages_name_not_blank_check CHECK (
		char_length(btrim(name)) > 0
	),
	CONSTRAINT crm_pipeline_stages_position_check CHECK (position >= 0),
	CONSTRAINT crm_pipeline_stages_probability_check CHECK (
		probability >= 0 AND probability <= 100
	),
	CONSTRAINT crm_pipeline_stages_won_lost_exclusive_check CHECK (
		NOT (is_won AND is_lost)
	),
	CONSTRAINT fk_crm_pipeline_stages_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_pipeline_stages_pipeline
		FOREIGN KEY (organization_id, pipeline_id)
		REFERENCES crm_pipelines(organization_id, id)
		ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_crm_pipeline_stages_pipeline_position_active_unique
	ON crm_pipeline_stages(pipeline_id, position)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_pipeline_stages_pipeline
	ON crm_pipeline_stages(organization_id, pipeline_id)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_pipeline_stages_deleted_at
	ON crm_pipeline_stages(deleted_at);

SELECT apply_organization_rls('crm_pipeline_stages'::regclass);
