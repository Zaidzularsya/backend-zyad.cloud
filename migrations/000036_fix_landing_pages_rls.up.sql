BEGIN;

-- Drop the default restrictive isolation boundary for landing_pages
DROP POLICY IF EXISTS organization_isolation_boundary ON landing_pages;
CREATE POLICY organization_isolation_boundary ON landing_pages
AS RESTRICTIVE FOR ALL TO PUBLIC
USING (
    app_organization_matches(organization_id) 
    OR visibility = 'public' 
    OR organization_id IS NULL
)
WITH CHECK (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
);

-- Drop the default permissive tenant access for landing_pages
DROP POLICY IF EXISTS organization_tenant_access ON landing_pages;
CREATE POLICY organization_tenant_access ON landing_pages
AS PERMISSIVE FOR ALL TO PUBLIC
USING (
    app_organization_matches(organization_id) 
    OR visibility = 'public' 
    OR organization_id IS NULL
)
WITH CHECK (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
);

-- Fix landing_brandings
DROP POLICY IF EXISTS organization_isolation_boundary ON landing_brandings;
CREATE POLICY organization_isolation_boundary ON landing_brandings
AS RESTRICTIVE FOR ALL TO PUBLIC
USING (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
)
WITH CHECK (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
);

DROP POLICY IF EXISTS organization_tenant_access ON landing_brandings;
CREATE POLICY organization_tenant_access ON landing_brandings
AS PERMISSIVE FOR ALL TO PUBLIC
USING (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
)
WITH CHECK (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
);

-- Fix landing_domain_bindings (public domains)
DROP POLICY IF EXISTS organization_isolation_boundary ON landing_domain_bindings;
CREATE POLICY organization_isolation_boundary ON landing_domain_bindings
AS RESTRICTIVE FOR ALL TO PUBLIC
USING (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
)
WITH CHECK (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
);

DROP POLICY IF EXISTS organization_tenant_access ON landing_domain_bindings;
CREATE POLICY organization_tenant_access ON landing_domain_bindings
AS PERMISSIVE FOR ALL TO PUBLIC
USING (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
)
WITH CHECK (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
);

-- Fix landing_page_sections
DROP POLICY IF EXISTS organization_isolation_boundary ON landing_page_sections;
CREATE POLICY organization_isolation_boundary ON landing_page_sections
AS RESTRICTIVE FOR ALL TO PUBLIC
USING (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
    OR EXISTS (
        SELECT 1 FROM landing_pages p 
        WHERE p.id = landing_page_sections.landing_page_id AND p.visibility = 'public'
    )
)
WITH CHECK (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
);

DROP POLICY IF EXISTS organization_tenant_access ON landing_page_sections;
CREATE POLICY organization_tenant_access ON landing_page_sections
AS PERMISSIVE FOR ALL TO PUBLIC
USING (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
    OR EXISTS (
        SELECT 1 FROM landing_pages p 
        WHERE p.id = landing_page_sections.landing_page_id AND p.visibility = 'public'
    )
)
WITH CHECK (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
);

-- Fix landing_page_versions (snapshots)
DROP POLICY IF EXISTS organization_isolation_boundary ON landing_page_versions;
CREATE POLICY organization_isolation_boundary ON landing_page_versions
AS RESTRICTIVE FOR ALL TO PUBLIC
USING (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
    OR EXISTS (
        SELECT 1 FROM landing_pages p 
        WHERE p.id = landing_page_versions.landing_page_id AND p.visibility = 'public'
    )
)
WITH CHECK (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
);

DROP POLICY IF EXISTS organization_tenant_access ON landing_page_versions;
CREATE POLICY organization_tenant_access ON landing_page_versions
AS PERMISSIVE FOR ALL TO PUBLIC
USING (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
    OR EXISTS (
        SELECT 1 FROM landing_pages p 
        WHERE p.id = landing_page_versions.landing_page_id AND p.visibility = 'public'
    )
)
WITH CHECK (
    app_organization_matches(organization_id) 
    OR organization_id IS NULL
);

COMMIT;
