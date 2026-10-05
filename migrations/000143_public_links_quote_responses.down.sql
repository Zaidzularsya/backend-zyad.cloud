-- Kembalikan data ke bentuk yang diterima constraint lama sebelum constraint dipasang.
UPDATE crm_quotations SET status = 'sent' WHERE status = 'revision_requested';
UPDATE crm_activities SET type = 'note' WHERE type = 'quotation_response';

ALTER TABLE crm_activities DROP CONSTRAINT IF EXISTS crm_activities_type_check;
ALTER TABLE crm_activities ADD CONSTRAINT crm_activities_type_check CHECK (
	type IN ('call', 'email', 'meeting', 'task', 'note', 'whatsapp'));
ALTER TABLE crm_activities DROP COLUMN IF EXISTS metadata;

ALTER TABLE crm_quotations DROP CONSTRAINT IF EXISTS crm_quotations_status_check;
ALTER TABLE crm_quotations ADD CONSTRAINT crm_quotations_status_check CHECK (
	status IN ('draft', 'sent', 'approved', 'rejected', 'expired', 'superseded'));

DROP TABLE IF EXISTS crm_quotation_responses;
DROP TABLE IF EXISTS public_links;
