DROP POLICY IF EXISTS landing_section_templates_select_access ON landing_section_templates;
DROP POLICY IF EXISTS landing_section_templates_insert_access ON landing_section_templates;
DROP POLICY IF EXISTS landing_section_templates_update_access ON landing_section_templates;
DROP POLICY IF EXISTS landing_section_templates_delete_access ON landing_section_templates;

CREATE POLICY organization_isolation_boundary ON landing_section_templates
	AS RESTRICTIVE
	USING (app_organization_matches(organization_id))
	WITH CHECK (app_organization_matches(organization_id));

CREATE POLICY organization_tenant_access ON landing_section_templates
	USING (app_organization_matches(organization_id))
	WITH CHECK (app_organization_matches(organization_id));
