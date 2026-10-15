-- Day (Asia/Jakarta) on which the conversation last produced its CRM
-- timeline activity. The whatsapp module writes at most one 'whatsapp'
-- activity per conversation per day; claiming the day with a conditional
-- UPDATE on this column keeps that race-free.
ALTER TABLE wa_conversations
	ADD COLUMN IF NOT EXISTS crm_activity_on date;
