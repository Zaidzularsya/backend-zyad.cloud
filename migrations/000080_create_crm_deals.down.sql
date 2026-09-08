ALTER TABLE crm_leads DROP CONSTRAINT IF EXISTS fk_crm_leads_converted_deal;

SELECT remove_organization_rls('crm_deals'::regclass);

DROP TABLE IF EXISTS crm_deals;
