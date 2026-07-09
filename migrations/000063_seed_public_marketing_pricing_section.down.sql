WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
),
landing_page AS (
	SELECT id FROM landing_pages
	WHERE organization_id = (SELECT id FROM platform_org) AND slug = 'public-marketing' LIMIT 1
)
DELETE FROM landing_page_sections
WHERE landing_page_id = (SELECT id FROM landing_page) AND section_key = 'pricing-section';
