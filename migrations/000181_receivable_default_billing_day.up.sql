-- Tenant default for "tanggal tagih setiap bulan" (day of month 1–31) used to
-- prefill customer onboarding. NULL = the onboarding day.
ALTER TABLE receivable_settings
	ADD COLUMN IF NOT EXISTS default_billing_day smallint NULL
	CONSTRAINT receivable_settings_default_billing_day_check CHECK (default_billing_day BETWEEN 1 AND 31);
