ALTER TABLE landing_menus DISABLE ROW LEVEL SECURITY;
ALTER TABLE landing_menu_items DISABLE ROW LEVEL SECURITY;

WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
),
header_menu AS (
	SELECT id FROM landing_menus
	WHERE organization_id = (SELECT id FROM platform_org)
		AND location = 'header'
		AND is_active = true
		AND deleted_at IS NULL
	LIMIT 1
)
UPDATE landing_menu_items
SET label = 'Benefits', updated_at = now()
WHERE menu_id = (SELECT id FROM header_menu)
	AND destination = '#benefits';

WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
)
UPDATE landing_menus
SET deleted_at = NULL, is_active = true, updated_at = now()
WHERE organization_id = (SELECT id FROM platform_org)
	AND name = 'tes';

ALTER TABLE landing_menu_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE landing_menus ENABLE ROW LEVEL SECURITY;
