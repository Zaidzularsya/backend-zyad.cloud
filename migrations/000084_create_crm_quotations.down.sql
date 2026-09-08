SELECT remove_organization_rls('crm_quotation_items'::regclass);
SELECT remove_organization_rls('crm_quotations'::regclass);

DROP TABLE IF EXISTS crm_quotation_items;
DROP TABLE IF EXISTS crm_quotations;
