ALTER TABLE crm_companies
	ADD COLUMN IF NOT EXISTS tenant_organization_id uuid NULL REFERENCES organizations(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_crm_companies_tenant_org_unique
	ON crm_companies(organization_id, tenant_organization_id)
	WHERE tenant_organization_id IS NOT NULL AND deleted_at IS NULL;

ALTER TABLE crm_quotations
	ADD COLUMN IF NOT EXISTS channel varchar(20) NULL;

ALTER TABLE crm_quotations
	ADD CONSTRAINT crm_quotations_channel_check
		CHECK (channel IS NULL OR channel = 'self_serve');

CREATE UNIQUE INDEX IF NOT EXISTS idx_crm_quotations_self_serve_deal
	ON crm_quotations(organization_id, deal_id)
	WHERE channel = 'self_serve' AND status IN ('draft', 'approved') AND deleted_at IS NULL;

ALTER TABLE crm_pipelines
	ADD COLUMN IF NOT EXISTS system_key varchar(50) NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_crm_pipelines_system_key
	ON crm_pipelines(organization_id, system_key)
	WHERE system_key IS NOT NULL AND deleted_at IS NULL;
