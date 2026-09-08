SELECT remove_organization_rls('crm_invoice_items'::regclass);
SELECT remove_organization_rls('crm_invoices'::regclass);

DROP TABLE IF EXISTS crm_invoice_items;
DROP TABLE IF EXISTS crm_invoices;
