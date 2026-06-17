CREATE TABLE IF NOT EXISTS landing_page_versions (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	landing_page_id uuid NOT NULL,
	version integer NOT NULL,
	change_note text,
	snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT landing_page_versions_version_check CHECK (version > 0),
	CONSTRAINT landing_page_versions_snapshot_object_check CHECK (jsonb_typeof(snapshot) = 'object'),
	CONSTRAINT fk_landing_page_versions_page
		FOREIGN KEY (organization_id, landing_page_id)
		REFERENCES landing_pages(organization_id, id)
		ON DELETE CASCADE,
	CONSTRAINT fk_landing_page_versions_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_page_versions_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_page_versions_page_ver_unique
	ON landing_page_versions(organization_id, landing_page_id, version);

CREATE INDEX IF NOT EXISTS idx_landing_page_versions_page
	ON landing_page_versions(organization_id, landing_page_id);


CREATE TABLE IF NOT EXISTS landing_slug_redirects (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	source_slug varchar(120) NOT NULL,
	target_slug varchar(120) NOT NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT landing_slug_redirects_prevent_loop_check CHECK (source_slug <> target_slug),
	CONSTRAINT landing_slug_redirects_source_format_check CHECK (
		source_slug = lower(source_slug)
		AND source_slug ~ '^[a-z0-9]([a-z0-9-]{0,118}[a-z0-9])?$'
	),
	CONSTRAINT landing_slug_redirects_target_format_check CHECK (
		target_slug = lower(target_slug)
		AND target_slug ~ '^[a-z0-9]([a-z0-9-]{0,118}[a-z0-9])?$'
	),
	CONSTRAINT fk_landing_slug_redirects_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_slug_redirects_org_source_unique
	ON landing_slug_redirects(organization_id, lower(source_slug));

SELECT apply_organization_rls('landing_page_versions'::regclass);
SELECT apply_organization_rls('landing_slug_redirects'::regclass);
