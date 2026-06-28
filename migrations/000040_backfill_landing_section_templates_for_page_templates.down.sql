ALTER TABLE landing_pages DISABLE ROW LEVEL SECURITY;
ALTER TABLE landing_page_sections DISABLE ROW LEVEL SECURITY;
ALTER TABLE landing_section_templates DISABLE ROW LEVEL SECURITY;

WITH template_pages AS (
	SELECT id, organization_id, slug
	FROM landing_pages
	WHERE deleted_at IS NULL
		AND (
			is_template = true
			OR settings->>'kind' = 'landing_page_template'
		)
),
template_section_keys AS (
	SELECT
		t.organization_id,
		t.content->'metadata'->>'templateSlug' AS slug,
		t.content->'metadata'->>'sectionKey' AS section_key,
		t.id::text AS template_id
	FROM landing_section_templates t
	JOIN template_pages p
		ON p.organization_id = t.organization_id
		AND p.slug = t.content->'metadata'->>'templateSlug'
	WHERE t.deleted_at IS NULL
)
UPDATE landing_page_sections s
SET
	content = s.content - 'source',
	updated_at = now()
FROM template_pages p
JOIN template_section_keys k
	ON k.organization_id = p.organization_id
	AND k.slug = p.slug
WHERE s.organization_id = p.organization_id
	AND s.landing_page_id = p.id
	AND s.section_key = k.section_key
	AND s.content->'source'->>'sectionTemplateId' = k.template_id;

WITH template_pages AS (
	SELECT organization_id, slug
	FROM landing_pages
	WHERE deleted_at IS NULL
		AND (
			is_template = true
			OR settings->>'kind' = 'landing_page_template'
		)
)
DELETE FROM landing_section_templates t
USING template_pages p
WHERE t.organization_id = p.organization_id
	AND t.content->'metadata'->>'templateSlug' = p.slug;

ALTER TABLE landing_pages ENABLE ROW LEVEL SECURITY;
ALTER TABLE landing_page_sections ENABLE ROW LEVEL SECURITY;
ALTER TABLE landing_section_templates ENABLE ROW LEVEL SECURITY;
