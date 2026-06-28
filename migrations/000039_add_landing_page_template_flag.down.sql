ALTER TABLE landing_pages DISABLE ROW LEVEL SECURITY;

DROP INDEX IF EXISTS idx_landing_pages_organization_template;

ALTER TABLE landing_pages
	DROP COLUMN IF EXISTS is_template;

ALTER TABLE landing_pages ENABLE ROW LEVEL SECURITY;
