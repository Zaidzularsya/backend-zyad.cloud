-- Revert the Zyad Technovation rebrand: clear logo assets and legal contact details on the
-- platform branding row and remove the "Legal" footer column + legal-entity copyright,
-- restoring the 000065 footer state.
ALTER TABLE landing_brandings DISABLE ROW LEVEL SECURITY;

WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
)
UPDATE landing_brandings b
SET
	logo_light_url = NULL,
	logo_dark_url = NULL,
	favicon_url = NULL,
	contact = b.contact || jsonb_build_object(
		'email', '',
		'address', 'Indonesia'
	),
	updated_at = now()
FROM platform_org o
WHERE b.organization_id = o.id
	AND b.landing_page_id IS NULL
	AND b.company_name = 'Zyad Cloud';

ALTER TABLE landing_brandings ENABLE ROW LEVEL SECURITY;

ALTER TABLE landing_page_sections DISABLE ROW LEVEL SECURITY;

WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
),
landing_page AS (
	SELECT id FROM landing_pages
	WHERE organization_id = (SELECT id FROM platform_org) AND slug = 'public-marketing'
	LIMIT 1
)
UPDATE landing_page_sections s
SET
	content = jsonb_set(
		s.content || jsonb_build_object('copyright', '© 2026 Zyad Cloud. All rights reserved.'),
		'{columns}',
		COALESCE(
			(
				SELECT jsonb_agg(col)
				FROM jsonb_array_elements(COALESCE(s.content->'columns', '[]'::jsonb)) AS col
				WHERE col->>'title' <> 'Legal'
			),
			'[]'::jsonb
		)
	),
	updated_at = now()
FROM landing_page lp
WHERE s.landing_page_id = lp.id
	AND s.section_type = 'footer'
	AND s.deleted_at IS NULL;

ALTER TABLE landing_page_sections ENABLE ROW LEVEL SECURITY;
