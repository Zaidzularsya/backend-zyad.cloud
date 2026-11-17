-- Gelombang 3: recording old arrears (tunggakan) as back-dated invoices.
-- One recording = one admin action with a reason; it owns 1..24 invoices
-- (receivable_invoices.arrears_recording_id). The idempotency key comes from
-- the client (one per dialog) so a double submit returns the first recording.
CREATE TABLE IF NOT EXISTS receivable_arrears_recordings (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	account_id uuid NOT NULL,
	contract_id uuid NOT NULL,
	reason varchar(500) NOT NULL,
	recorded_by uuid NULL,
	idempotency_key varchar(100) NOT NULL,
	notify_status varchar(20) NOT NULL DEFAULT 'pending',
	notify_error text NOT NULL DEFAULT '',
	notified_at timestamp without time zone NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT receivable_arrears_recordings_org_id_unique UNIQUE (organization_id, id),
	CONSTRAINT receivable_arrears_recordings_key_unique UNIQUE (organization_id, idempotency_key),
	CONSTRAINT receivable_arrears_recordings_notify_status_check CHECK (notify_status IN ('pending', 'sent', 'failed', 'skipped')),
	CONSTRAINT fk_receivable_arrears_recordings_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_receivable_arrears_recordings_account FOREIGN KEY (organization_id, account_id)
		REFERENCES receivable_accounts(organization_id, id) ON DELETE RESTRICT,
	CONSTRAINT fk_receivable_arrears_recordings_contract FOREIGN KEY (organization_id, contract_id)
		REFERENCES receivable_contracts(organization_id, id) ON DELETE RESTRICT,
	CONSTRAINT fk_receivable_arrears_recordings_recorded_by FOREIGN KEY (recorded_by) REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_receivable_arrears_recordings_contract
	ON receivable_arrears_recordings(organization_id, contract_id, created_at);
SELECT apply_organization_rls('receivable_arrears_recordings'::regclass);

-- isolir_flag: the owner's explicit choice that this invoice may trigger
-- Mikrotik isolation (only arrears invoices use it; default off).
ALTER TABLE receivable_invoices
	ADD COLUMN IF NOT EXISTS isolir_flag boolean NOT NULL DEFAULT false,
	ADD COLUMN IF NOT EXISTS arrears_recording_id uuid NULL;
ALTER TABLE receivable_invoices
	ADD CONSTRAINT fk_receivable_invoices_arrears_recording FOREIGN KEY (organization_id, arrears_recording_id)
		REFERENCES receivable_arrears_recordings(organization_id, id) ON DELETE RESTRICT;
CREATE INDEX IF NOT EXISTS idx_receivable_invoices_arrears_recording
	ON receivable_invoices(organization_id, arrears_recording_id) WHERE arrears_recording_id IS NOT NULL;
-- Backstop for two concurrent recordings of the same period: at most one open (non-void)
-- arrears invoice per contract period. Normal invoices (arrears_recording_id NULL) are not covered.
CREATE UNIQUE INDEX IF NOT EXISTS idx_receivable_invoices_arrears_period
	ON receivable_invoices(organization_id, contract_id, period_start)
	WHERE arrears_recording_id IS NOT NULL AND status <> 'void';

CREATE TABLE IF NOT EXISTS receivable_isolir_flag_audits (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	invoice_id uuid NOT NULL,
	old_value boolean NOT NULL,
	new_value boolean NOT NULL,
	changed_by uuid NULL,
	changed_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT fk_receivable_isolir_flag_audits_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_receivable_isolir_flag_audits_invoice FOREIGN KEY (organization_id, invoice_id)
		REFERENCES receivable_invoices(organization_id, id) ON DELETE CASCADE,
	CONSTRAINT fk_receivable_isolir_flag_audits_changed_by FOREIGN KEY (changed_by) REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_receivable_isolir_flag_audits_invoice
	ON receivable_isolir_flag_audits(organization_id, invoice_id, changed_at);
SELECT apply_organization_rls('receivable_isolir_flag_audits'::regclass);
