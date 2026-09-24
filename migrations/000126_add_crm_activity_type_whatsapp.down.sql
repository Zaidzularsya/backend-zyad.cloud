-- Intentionally fails if 'whatsapp' activities exist: rolling back must not
-- silently delete timeline data. Remove or retype those rows first.
ALTER TABLE crm_activities DROP CONSTRAINT IF EXISTS crm_activities_type_check;

ALTER TABLE crm_activities
	ADD CONSTRAINT crm_activities_type_check CHECK (
		type IN ('call', 'email', 'meeting', 'task', 'note')
	);
