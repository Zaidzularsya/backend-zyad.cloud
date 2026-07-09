ALTER TABLE landing_page_sections DISABLE ROW LEVEL SECURITY;

WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
),
landing_page AS (
	SELECT id, organization_id FROM landing_pages
	WHERE organization_id = (SELECT id FROM platform_org) AND slug = 'public-marketing'
	LIMIT 1
)
INSERT INTO landing_page_sections (
	organization_id, landing_page_id, section_key, section_type, name, sort_order, content
)
SELECT
	lp.organization_id,
	lp.id,
	'pricing-section',
	'pricing',
	'Pricing Section',
	85,
	'{"title": "Pilih Paket yang Sesuai", "source": "platform_catalog", "billingInterval": "monthly"}'::jsonb
FROM landing_page lp
ON CONFLICT (organization_id, landing_page_id, lower(section_key)) WHERE deleted_at IS NULL
DO UPDATE SET
	content = EXCLUDED.content,
	sort_order = EXCLUDED.sort_order,
	updated_at = now();

ALTER TABLE landing_page_sections ENABLE ROW LEVEL SECURITY;
