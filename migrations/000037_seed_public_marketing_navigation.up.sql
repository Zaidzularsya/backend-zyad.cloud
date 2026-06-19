ALTER TABLE landing_menus DISABLE ROW LEVEL SECURITY;
ALTER TABLE landing_menu_items DISABLE ROW LEVEL SECURITY;

WITH platform_org AS (
	SELECT id
	FROM organizations
	WHERE type = 'platform'
	ORDER BY created_at NULLS LAST
	LIMIT 1
),
existing_header_menu AS (
	SELECT id, organization_id
	FROM landing_menus
	WHERE organization_id = (SELECT id FROM platform_org)
		AND location = 'header'
		AND is_active = true
		AND deleted_at IS NULL
	LIMIT 1
),
created_header_menu AS (
	INSERT INTO landing_menus (organization_id, name, location, is_active)
	SELECT id, 'Main navigation', 'header', true
	FROM platform_org
	WHERE EXISTS (
		SELECT 1
		FROM landing_pages
		WHERE organization_id = (SELECT id FROM platform_org)
			AND slug = 'public-marketing'
			AND deleted_at IS NULL
	)
		AND NOT EXISTS (SELECT 1 FROM existing_header_menu)
	RETURNING id, organization_id
),
header_menu AS (
	SELECT id, organization_id FROM created_header_menu
),
menu_items (label, link_type, destination, target, sort_order) AS (
	VALUES
		('Solusi', 'anchor', '#solusi', 'self', 10),
		('Cara Kerja', 'anchor', '#cara-kerja', 'self', 20),
		('Demo', 'anchor', '#demo-portal', 'self', 30),
		('FAQ', 'anchor', '#faq', 'self', 40),
		('Partner Program', 'anchor', '#benefits', 'self', 50)
)
INSERT INTO landing_menu_items (
	organization_id, menu_id, label, link_type, destination, target, sort_order, is_enabled
)
SELECT
	hm.organization_id,
	hm.id,
	mi.label,
	mi.link_type,
	mi.destination,
	mi.target,
	mi.sort_order,
	true
FROM header_menu hm
CROSS JOIN menu_items mi
WHERE NOT EXISTS (
	SELECT 1
	FROM landing_menu_items existing
	WHERE existing.organization_id = hm.organization_id
		AND existing.menu_id = hm.id
		AND existing.destination = mi.destination
);

ALTER TABLE landing_menus ENABLE ROW LEVEL SECURITY;
ALTER TABLE landing_menu_items ENABLE ROW LEVEL SECURITY;
