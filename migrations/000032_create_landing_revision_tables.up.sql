CREATE TABLE IF NOT EXISTS landing_page_revisions (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	landing_page_id uuid NOT NULL,
	revision_number integer NOT NULL,
	snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
	change_note text,
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT landing_page_revisions_number_check CHECK (revision_number > 0),
	CONSTRAINT landing_page_revisions_snapshot_object_check CHECK (jsonb_typeof(snapshot) = 'object'),
	CONSTRAINT fk_landing_page_revisions_page
		FOREIGN KEY (organization_id, landing_page_id)
		REFERENCES landing_pages(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_page_revisions_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_page_revisions_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_page_revisions_page_rev_unique
	ON landing_page_revisions(organization_id, landing_page_id, revision_number);

CREATE INDEX IF NOT EXISTS idx_landing_page_revisions_page
	ON landing_page_revisions(organization_id, landing_page_id);


CREATE TABLE IF NOT EXISTS landing_page_schedules (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	landing_page_id uuid NOT NULL,
	action varchar(30) NOT NULL,
	scheduled_at timestamp without time zone NOT NULL,
	status varchar(30) NOT NULL DEFAULT 'pending',
	lock_id uuid,
	lock_expires_at timestamp without time zone,
	error_message text,
	attempts integer NOT NULL DEFAULT 0,
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT landing_page_schedules_action_check CHECK (action IN ('publish', 'unpublish')),
	CONSTRAINT landing_page_schedules_status_check CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
	CONSTRAINT landing_page_schedules_attempts_check CHECK (attempts >= 0),
	CONSTRAINT fk_landing_page_schedules_page
		FOREIGN KEY (organization_id, landing_page_id)
		REFERENCES landing_pages(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_page_schedules_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_page_schedules_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_landing_page_schedules_claim
	ON landing_page_schedules(status, scheduled_at)
	WHERE status IN ('pending', 'failed');

SELECT apply_organization_rls('landing_page_revisions'::regclass);
-- landing_page_schedules does not have RLS so background workers can claim schedules globally
