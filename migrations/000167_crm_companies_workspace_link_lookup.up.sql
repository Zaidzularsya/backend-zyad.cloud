-- Cross-org workspace link lookup on crm_companies under FORCE RLS.
--
-- The cross-org guard (one customer workspace linked to at most one CRM company across all
-- organizations) must find a link that lives in ANOTHER organization's CRM. The standard policies
-- only expose rows of app.organization_id, and a SECURITY DEFINER function does not help because
-- FORCE ROW LEVEL SECURITY applies to the table owner too.
--
-- This adds a narrow, SELECT-only exception: inside a transaction that sets
-- app.crm_workspace_link_lookup = '<workspace uuid>', rows that are linked to exactly that workspace
-- and not soft-deleted become readable. Writes stay strictly organization-scoped:
-- - the restrictive boundary keeps WITH CHECK strict, and
-- - the only permissive policy covering UPDATE/DELETE/INSERT is still organization_tenant_access.
-- Policy names are kept so remove_organization_rls('crm_companies') keeps working.

DROP POLICY IF EXISTS organization_isolation_boundary ON crm_companies;
CREATE POLICY organization_isolation_boundary ON crm_companies
	AS RESTRICTIVE FOR ALL TO PUBLIC
	USING (
		app_organization_matches(organization_id)
		OR (
			tenant_organization_id IS NOT NULL
			AND deleted_at IS NULL
			AND tenant_organization_id::text = current_setting('app.crm_workspace_link_lookup', true)
		)
	)
	WITH CHECK (app_organization_matches(organization_id));

DROP POLICY IF EXISTS crm_companies_workspace_link_lookup ON crm_companies;
CREATE POLICY crm_companies_workspace_link_lookup ON crm_companies
	AS PERMISSIVE FOR SELECT TO PUBLIC
	USING (
		tenant_organization_id IS NOT NULL
		AND deleted_at IS NULL
		AND tenant_organization_id::text = current_setting('app.crm_workspace_link_lookup', true)
	);

CREATE INDEX IF NOT EXISTS idx_crm_companies_tenant_org_lookup
	ON crm_companies(tenant_organization_id)
	WHERE tenant_organization_id IS NOT NULL AND deleted_at IS NULL;
