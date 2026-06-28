DROP POLICY IF EXISTS organization_isolation_boundary ON landing_section_templates;
DROP POLICY IF EXISTS organization_tenant_access ON landing_section_templates;

CREATE POLICY landing_section_templates_select_access ON landing_section_templates
	FOR SELECT
	USING (
		app_organization_matches(organization_id)
		OR organization_id = (
			SELECT id
			FROM organizations
			WHERE type = 'platform'
			ORDER BY created_at ASC
			LIMIT 1
		)
	);

CREATE POLICY landing_section_templates_insert_access ON landing_section_templates
	FOR INSERT
	WITH CHECK (app_organization_matches(organization_id));

CREATE POLICY landing_section_templates_update_access ON landing_section_templates
	FOR UPDATE
	USING (app_organization_matches(organization_id))
	WITH CHECK (app_organization_matches(organization_id));

CREATE POLICY landing_section_templates_delete_access ON landing_section_templates
	FOR DELETE
	USING (app_organization_matches(organization_id));
