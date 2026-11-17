-- Where a deal came from, so onboarding deals ("Impor PPPoE") can be told
-- apart and filtered. Empty string = created manually / by lead convert.
ALTER TABLE crm_deals ADD COLUMN IF NOT EXISTS source varchar(40) NOT NULL DEFAULT '';
