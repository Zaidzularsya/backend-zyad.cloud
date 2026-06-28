ALTER TABLE landing_brandings DISABLE ROW LEVEL SECURITY;

WITH platform_landing_page AS (
	SELECT
		p.organization_id
	FROM landing_pages p
	JOIN organizations o ON o.id = p.organization_id
	WHERE o.type = 'platform'
		AND p.slug = 'public-marketing'
		AND p.deleted_at IS NULL
	ORDER BY p.created_at NULLS LAST
	LIMIT 1
)
DELETE FROM landing_brandings b
USING platform_landing_page p
WHERE b.organization_id = p.organization_id
	AND b.landing_page_id IS NULL
	AND b.company_name = 'HEY Digital Solution'
	AND b.tagline = 'Partner eksekusi teknologi terpercaya untuk B2B'
	AND b.contact = '{
		"email": "",
		"phone": "6281223329453",
		"address": "Indonesia"
	}'::jsonb
	AND b.social_links = '[
		{
			"platform": "whatsapp",
			"url": "https://wa.me/6281223329453"
		}
	]'::jsonb;

ALTER TABLE landing_brandings ENABLE ROW LEVEL SECURITY;
