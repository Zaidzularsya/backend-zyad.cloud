SELECT remove_organization_rls('landing_page_sections'::regclass);
SELECT remove_organization_rls('landing_pages'::regclass);

DROP TABLE IF EXISTS landing_page_sections;
DROP TABLE IF EXISTS landing_pages;
