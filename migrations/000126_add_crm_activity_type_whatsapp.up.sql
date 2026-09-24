-- WhatsApp conversations are recorded on the CRM timeline as activity type
-- 'whatsapp' (one summary activity per conversation per day, written by the
-- whatsapp module).
ALTER TABLE crm_activities DROP CONSTRAINT IF EXISTS crm_activities_type_check;

ALTER TABLE crm_activities
	ADD CONSTRAINT crm_activities_type_check CHECK (
		type IN ('call', 'email', 'meeting', 'task', 'note', 'whatsapp')
	);
