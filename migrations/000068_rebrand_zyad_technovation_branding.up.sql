-- Rebrand the platform branding from the legacy "HEY Digital Solution" agency identity to
-- Zyad Cloud (product) operated by PT Zyad Technovation Indonesia (legal entity), aligned
-- with the official legal documents (Terms and Conditions, Privacy Policy, Acceptable Use
-- Policy v1.0):
--  * Set company_name/tagline, official logo assets (served statically by the frontend from
--    /branding/*), and the official contact email/address on the platform-level branding row.
--  * Add a "Legal" footer column on the public-marketing page linking the legal documents
--    (/legal/terms, /legal/privacy, /legal/refund, /legal/aup) and update the copyright to
--    name the legal entity.
ALTER TABLE landing_brandings DISABLE ROW LEVEL SECURITY;

WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
)
UPDATE landing_brandings b
SET
	company_name = 'Zyad Cloud',
	tagline = 'IT Solutions · Web · SaaS Multi-Tenant',
	logo_light_url = '/branding/zyad-logo-light.svg',
	logo_dark_url = '/branding/zyad-logo-dark.svg',
	favicon_url = '/branding/favicon.svg',
	contact = b.contact || jsonb_build_object(
		'email', 'yulianto.personal@outlook.com',
		'address', 'Desa Lumpang, Kec. Karanganyar, Kab. Purbalingga, Jawa Tengah'
	),
	updated_at = now()
FROM platform_org o
WHERE b.organization_id = o.id
	AND b.landing_page_id IS NULL;

ALTER TABLE landing_brandings ENABLE ROW LEVEL SECURITY;

ALTER TABLE landing_page_sections DISABLE ROW LEVEL SECURITY;

WITH platform_org AS (
	SELECT id FROM organizations WHERE type = 'platform' LIMIT 1
),
landing_page AS (
	SELECT id FROM landing_pages
	WHERE organization_id = (SELECT id FROM platform_org) AND slug = 'public-marketing'
	LIMIT 1
)
UPDATE landing_page_sections s
SET
	content = s.content || jsonb_build_object(
		'copyright', '© 2026 Zyad Cloud — PT Zyad Technovation Indonesia. All rights reserved.',
		'columns', COALESCE(s.content->'columns', '[]'::jsonb) || '[
			{"title": "Legal", "links": [
				{"label": "Syarat & Ketentuan", "href": "/legal/terms"},
				{"label": "Kebijakan Privasi", "href": "/legal/privacy"},
				{"label": "Kebijakan Refund", "href": "/legal/refund"},
				{"label": "Acceptable Use Policy", "href": "/legal/aup"}
			]}
		]'::jsonb
	),
	updated_at = now()
FROM landing_page lp
WHERE s.landing_page_id = lp.id
	AND s.section_type = 'footer'
	AND s.deleted_at IS NULL
	AND NOT COALESCE(s.content->'columns', '[]'::jsonb) @> '[{"title": "Legal"}]'::jsonb;

ALTER TABLE landing_page_sections ENABLE ROW LEVEL SECURITY;
