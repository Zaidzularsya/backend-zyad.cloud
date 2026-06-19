BEGIN;

SELECT apply_organization_rls('landing_pages'::regclass);
SELECT apply_organization_rls('landing_brandings'::regclass);
SELECT apply_organization_rls('landing_domain_bindings'::regclass);
SELECT apply_organization_rls('landing_page_sections'::regclass);
SELECT apply_organization_rls('landing_page_versions'::regclass);

COMMIT;
