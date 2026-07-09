-- Republish the platform's public-marketing landing page snapshot so the newly seeded
-- pricing section (000063) is served by the public resolve endpoint, which reads from the
-- latest landing_page_versions snapshot rather than live landing_page_sections rows once a
-- page has been published at least once.
ALTER TABLE landing_page_versions DISABLE ROW LEVEL SECURITY;

WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
),
landing_page AS (
	SELECT id, organization_id FROM landing_pages
	WHERE organization_id = (SELECT id FROM platform_org) AND slug = 'public-marketing'
	LIMIT 1
),
next_version AS (
	SELECT COALESCE(MAX(version), 0) + 1 AS version
	FROM landing_page_versions
	WHERE landing_page_id = (SELECT id FROM landing_page)
)
INSERT INTO landing_page_versions (
	organization_id, landing_page_id, version, change_note, snapshot
)
SELECT
	lp.organization_id,
	lp.id,
	nv.version,
	'Seed: republish with platform catalog pricing section',
	jsonb_build_object(
		'page', (SELECT to_jsonb(p) FROM landing_pages p WHERE p.id = lp.id),
		'sections', COALESCE(
			(
				SELECT jsonb_agg(to_jsonb(s) ORDER BY s.sort_order)
				FROM landing_page_sections s
				WHERE s.landing_page_id = lp.id AND s.deleted_at IS NULL
			),
			'[]'::jsonb
		),
		'forms', COALESCE(
			(
				SELECT jsonb_agg(to_jsonb(f))
				FROM landing_forms f
				WHERE f.landing_page_id = lp.id AND f.deleted_at IS NULL
			),
			'[]'::jsonb
		),
		'snapshot_time', to_jsonb(now())
	)
FROM landing_page lp
CROSS JOIN next_version nv;

ALTER TABLE landing_page_versions ENABLE ROW LEVEL SECURITY;
