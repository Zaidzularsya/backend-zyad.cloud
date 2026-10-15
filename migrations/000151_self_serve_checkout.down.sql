DROP INDEX IF EXISTS idx_crm_pipelines_system_key;
ALTER TABLE crm_pipelines DROP COLUMN IF EXISTS system_key;

DROP INDEX IF EXISTS idx_crm_quotations_self_serve_deal;
ALTER TABLE crm_quotations DROP CONSTRAINT IF EXISTS crm_quotations_channel_check;
ALTER TABLE crm_quotations DROP COLUMN IF EXISTS channel;

DROP INDEX IF EXISTS idx_crm_companies_tenant_org_unique;
ALTER TABLE crm_companies DROP COLUMN IF EXISTS tenant_organization_id;
