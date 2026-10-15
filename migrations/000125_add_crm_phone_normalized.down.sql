DROP INDEX IF EXISTS idx_crm_contacts_organization_phone_normalized;
DROP INDEX IF EXISTS idx_crm_leads_organization_phone_normalized;

ALTER TABLE crm_contacts DROP COLUMN IF EXISTS phone_normalized;
ALTER TABLE crm_leads DROP COLUMN IF EXISTS phone_normalized;

DROP FUNCTION IF EXISTS normalize_phone_id(text);
