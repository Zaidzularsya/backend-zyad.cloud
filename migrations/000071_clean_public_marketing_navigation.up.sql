-- Clean up the platform's public-marketing header navigation: remove leftover test
-- entries created ad-hoc via the admin panel ("tes" item linking to example.com, and a
-- whole orphan "tes" menu with a "teslabel" item), and rename the English "Benefits"
-- label to the Indonesian "Manfaat" to match the rest of the header nav's tone
-- (Solusi, Cara Kerja, Demo Portal, FAQ).
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
DELETE FROM landing_menu_items
WHERE menu_id = (SELECT id FROM header_menu)
	AND link_type = 'external_link'
	AND destination = 'https://example.com';

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
SET label = 'Manfaat', updated_at = now()
WHERE menu_id = (SELECT id FROM header_menu)
	AND destination = '#benefits';

WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
),
orphan_test_menu AS (
	SELECT id FROM landing_menus
	WHERE organization_id = (SELECT id FROM platform_org)
		AND name = 'tes'
		AND deleted_at IS NULL
	LIMIT 1
)
DELETE FROM landing_menu_items
WHERE menu_id = (SELECT id FROM orphan_test_menu);

WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
)
UPDATE landing_menus
SET deleted_at = now(), is_active = false, updated_at = now()
WHERE organization_id = (SELECT id FROM platform_org)
	AND name = 'tes'
	AND deleted_at IS NULL;

ALTER TABLE landing_menu_items ENABLE ROW LEVEL SECURITY;
ALTER TABLE landing_menus ENABLE ROW LEVEL SECURITY;
