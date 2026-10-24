SELECT remove_organization_rls('landing_page_activations'::regclass);
DROP TABLE IF EXISTS landing_page_activations;
ALTER TABLE landing_pages DROP COLUMN IF EXISTS active_until;
