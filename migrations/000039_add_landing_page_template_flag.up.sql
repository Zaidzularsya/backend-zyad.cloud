ALTER TABLE landing_pages DISABLE ROW LEVEL SECURITY;

ALTER TABLE landing_pages
	ADD COLUMN IF NOT EXISTS is_template boolean NOT NULL DEFAULT false;

UPDATE landing_pages
SET is_template = true,
	updated_at = now()
WHERE settings->>'kind' = 'landing_page_template'
	AND is_template = false;

CREATE INDEX IF NOT EXISTS idx_landing_pages_organization_template
	ON landing_pages(organization_id, is_template, page_type)
	WHERE deleted_at IS NULL;

ALTER TABLE landing_pages ENABLE ROW LEVEL SECURITY;
