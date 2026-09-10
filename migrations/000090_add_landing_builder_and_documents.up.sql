-- Introduces the GrapesJS builder as a per-page alternative to the section model.
--   landing_pages.builder      : which editor authors this page ('sections' legacy, 'grapesjs' new)
--   landing_page_documents     : 1:1 store for a GrapesJS page — the editable project
--                                JSON plus its last-saved HTML/CSS export (working copy).
-- Publish snapshots still live in landing_page_versions.snapshot (no change there).

BEGIN;

ALTER TABLE landing_pages
	ADD COLUMN IF NOT EXISTS builder varchar(20) NOT NULL DEFAULT 'sections';

ALTER TABLE landing_pages
	DROP CONSTRAINT IF EXISTS landing_pages_builder_check;
ALTER TABLE landing_pages
	ADD CONSTRAINT landing_pages_builder_check CHECK (builder IN ('sections', 'grapesjs'));

CREATE TABLE IF NOT EXISTS landing_page_documents (
	landing_page_id uuid PRIMARY KEY,
	organization_id uuid NOT NULL,
	project jsonb NOT NULL DEFAULT '{}'::jsonb,
	html text NOT NULL DEFAULT '',
	css text NOT NULL DEFAULT '',
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT landing_page_documents_project_object_check CHECK (jsonb_typeof(project) = 'object'),
	CONSTRAINT fk_landing_page_documents_page
		FOREIGN KEY (organization_id, landing_page_id)
		REFERENCES landing_pages(organization_id, id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_page_documents_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_landing_page_documents_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

SELECT apply_organization_rls('landing_page_documents'::regclass);

-- Public-read carve-out (mirrors migration 000036 for landing_page_sections):
-- the unauthenticated public resolver runs with the visitor's resolved tenant as
-- the session org, so it must still be able to SELECT the document of a page whose
-- visibility is 'public'. Writes stay strictly tenant-scoped.
DROP POLICY IF EXISTS organization_isolation_boundary ON landing_page_documents;
CREATE POLICY organization_isolation_boundary ON landing_page_documents
AS RESTRICTIVE FOR ALL TO PUBLIC
USING (
	app_organization_matches(organization_id)
	OR organization_id IS NULL
	OR EXISTS (
		SELECT 1 FROM landing_pages p
		WHERE p.id = landing_page_documents.landing_page_id AND p.visibility = 'public'
	)
)
WITH CHECK (
	app_organization_matches(organization_id)
	OR organization_id IS NULL
);

DROP POLICY IF EXISTS organization_tenant_access ON landing_page_documents;
CREATE POLICY organization_tenant_access ON landing_page_documents
AS PERMISSIVE FOR ALL TO PUBLIC
USING (
	app_organization_matches(organization_id)
	OR organization_id IS NULL
	OR EXISTS (
		SELECT 1 FROM landing_pages p
		WHERE p.id = landing_page_documents.landing_page_id AND p.visibility = 'public'
	)
)
WITH CHECK (
	app_organization_matches(organization_id)
	OR organization_id IS NULL
);

COMMIT;
