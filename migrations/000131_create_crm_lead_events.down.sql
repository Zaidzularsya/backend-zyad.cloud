DROP INDEX IF EXISTS idx_crm_leads_organization_converted_at;
DROP INDEX IF EXISTS idx_crm_leads_organization_created_at;

SELECT remove_organization_rls('crm_lead_events'::regclass);

DROP TABLE IF EXISTS crm_lead_events;
