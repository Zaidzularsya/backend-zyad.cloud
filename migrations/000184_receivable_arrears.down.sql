DROP TABLE IF EXISTS receivable_isolir_flag_audits;
DROP INDEX IF EXISTS idx_receivable_invoices_arrears_recording;
ALTER TABLE receivable_invoices DROP CONSTRAINT IF EXISTS fk_receivable_invoices_arrears_recording;
ALTER TABLE receivable_invoices DROP COLUMN IF EXISTS arrears_recording_id, DROP COLUMN IF EXISTS isolir_flag;
DROP TABLE IF EXISTS receivable_arrears_recordings;
