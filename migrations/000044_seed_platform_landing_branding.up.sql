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
),
branding_seed AS (
	SELECT
		organization_id,
		'HEY Digital Solution'::varchar(200) AS company_name,
		'Partner eksekusi teknologi terpercaya untuk B2B'::varchar(255) AS tagline,
		'{
			"primary": "#2563EB",
			"secondary": "#0F172A",
			"accent": "#F59E0B",
			"background": "#FFFFFF",
			"surface": "#F8FAFC",
			"text": "#0F172A",
			"muted": "#64748B"
		}'::jsonb AS colors,
		'{
			"heading_font": "Inter",
			"body_font": "Inter"
		}'::jsonb AS typography,
		'{
			"button_radius": "8px",
			"card_radius": "12px"
		}'::jsonb AS shape,
		'{
			"width": "wide",
			"spacing": "comfortable",
			"background_style": "solid",
			"color_mode": "system",
			"header_style": "default",
			"footer_style": "default"
		}'::jsonb AS layout,
		'{
			"email": "",
			"phone": "6281223329453",
			"address": "Indonesia"
		}'::jsonb AS contact,
		'[
			{
				"platform": "whatsapp",
				"url": "https://wa.me/6281223329453"
			}
		]'::jsonb AS social_links
	FROM platform_landing_page
)
INSERT INTO landing_brandings (
	organization_id,
	landing_page_id,
	company_name,
	tagline,
	colors,
	typography,
	shape,
	layout,
	contact,
	social_links
)
SELECT
	organization_id,
	NULL,
	company_name,
	tagline,
	colors,
	typography,
	shape,
	layout,
	contact,
	social_links
FROM branding_seed
ON CONFLICT (organization_id) WHERE landing_page_id IS NULL
DO UPDATE SET
	company_name = COALESCE(NULLIF(landing_brandings.company_name, ''), EXCLUDED.company_name),
	tagline = COALESCE(NULLIF(landing_brandings.tagline, ''), EXCLUDED.tagline),
	colors = CASE
		WHEN landing_brandings.colors = '{}'::jsonb THEN EXCLUDED.colors
		ELSE landing_brandings.colors
	END,
	typography = CASE
		WHEN landing_brandings.typography = '{}'::jsonb THEN EXCLUDED.typography
		ELSE landing_brandings.typography
	END,
	shape = CASE
		WHEN landing_brandings.shape = '{}'::jsonb THEN EXCLUDED.shape
		ELSE landing_brandings.shape
	END,
	layout = CASE
		WHEN landing_brandings.layout = '{}'::jsonb THEN EXCLUDED.layout
		ELSE landing_brandings.layout
	END,
	contact = CASE
		WHEN landing_brandings.contact = '{}'::jsonb THEN EXCLUDED.contact
		ELSE landing_brandings.contact
	END,
	social_links = CASE
		WHEN landing_brandings.social_links = '[]'::jsonb THEN EXCLUDED.social_links
		ELSE landing_brandings.social_links
	END,
	updated_at = now();

ALTER TABLE landing_brandings ENABLE ROW LEVEL SECURITY;
