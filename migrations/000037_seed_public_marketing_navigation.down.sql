ALTER TABLE landing_menus DISABLE ROW LEVEL SECURITY;
ALTER TABLE landing_menu_items DISABLE ROW LEVEL SECURITY;

WITH platform_org AS (
	SELECT id
	FROM organizations
	WHERE type = 'platform'
	ORDER BY created_at NULLS LAST
	LIMIT 1
),
seeded_menu AS (
	SELECT id, organization_id
	FROM landing_menus
	WHERE organization_id = (SELECT id FROM platform_org)
		AND name = 'Main navigation'
		AND location = 'header'
)
DELETE FROM landing_menu_items item
USING seeded_menu menu
WHERE item.organization_id = menu.organization_id
	AND item.menu_id = menu.id
	AND item.destination IN ('#solusi', '#cara-kerja', '#demo-portal', '#faq', '#benefits');

DELETE FROM landing_menus menu
USING platform_org org
WHERE menu.organization_id = org.id
	AND menu.name = 'Main navigation'
	AND menu.location = 'header'
	AND NOT EXISTS (
		SELECT 1
		FROM landing_menu_items item
		WHERE item.organization_id = menu.organization_id
			AND item.menu_id = menu.id
	);

ALTER TABLE landing_menus ENABLE ROW LEVEL SECURITY;
ALTER TABLE landing_menu_items ENABLE ROW LEVEL SECURITY;
