-- Directory sesi checkout DOKU untuk invoice receivable. Tanpa RLS: webhook
-- harus menemukan organisasi dari nomor DOKU sebelum scope diketahui
-- (pola mailbox_directory / public_links). Operasi ber-scope memfilter organization_id.
CREATE TABLE IF NOT EXISTS receivable_checkouts (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
	invoice_id uuid NOT NULL,
	doku_invoice_number varchar(64) NOT NULL,
	amount numeric(18, 2) NOT NULL,
	payment_url text NOT NULL,
	status varchar(20) NOT NULL DEFAULT 'pending',
	expires_at timestamp without time zone NOT NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT receivable_checkouts_status_check CHECK (status IN ('pending', 'paid', 'expired', 'failed')),
	CONSTRAINT fk_receivable_checkouts_invoice FOREIGN KEY (organization_id, invoice_id)
		REFERENCES receivable_invoices(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_receivable_checkouts_number ON receivable_checkouts(doku_invoice_number, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_receivable_checkouts_invoice ON receivable_checkouts(organization_id, invoice_id, created_at DESC);
