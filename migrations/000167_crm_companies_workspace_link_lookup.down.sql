DROP INDEX IF EXISTS idx_crm_companies_tenant_org_lookup;

DROP POLICY IF EXISTS crm_companies_workspace_link_lookup ON crm_companies;

DROP POLICY IF EXISTS organization_isolation_boundary ON crm_companies;
CREATE POLICY organization_isolation_boundary ON crm_companies
	AS RESTRICTIVE FOR ALL TO PUBLIC
	USING (app_organization_matches(organization_id))
	WITH CHECK (app_organization_matches(organization_id));
