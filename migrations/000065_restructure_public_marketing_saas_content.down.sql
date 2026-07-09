-- Content rewrite is not restored (previous copy was legacy seed data); this down migration
-- only removes the section added by the up migration so the page keeps rendering.
ALTER TABLE landing_page_sections DISABLE ROW LEVEL SECURITY;

WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
),
landing_page AS (
	SELECT id FROM landing_pages
	WHERE organization_id = (SELECT id FROM platform_org) AND slug = 'public-marketing' LIMIT 1
)
DELETE FROM landing_page_sections
WHERE landing_page_id = (SELECT id FROM landing_page) AND section_key = 'statistics-section';

ALTER TABLE landing_page_sections ENABLE ROW LEVEL SECURITY;
