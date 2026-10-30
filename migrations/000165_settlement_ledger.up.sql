-- Ledger penyelesaian: hak tiap organisasi (tenant/reseller) atas uang yang masuk
-- merchant Zyad lewat DOKU, plus setoran & penyesuaian manual (Task 16).
-- Pembukuan lintas tenant milik platform, BUKAN data tenant: sengaja TANPA
-- apply_organization_rls. Akses dibatasi di service layer.
-- FK organisasi tanpa ON DELETE CASCADE: catatan uang tidak boleh ikut terhapus.
-- invoice_id tanpa FK: entri tetap ada walau invoice dihapus/berubah.
CREATE TABLE IF NOT EXISTS settlement_entries (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	beneficiary_organization_id uuid NOT NULL,
	kind varchar(20) NOT NULL,
	amount numeric(20, 2) NOT NULL,
	invoice_id uuid NULL,
	provider_reference text NULL,
	note text NOT NULL DEFAULT '',
	occurred_at timestamp without time zone NOT NULL,
	created_by uuid NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT settlement_entries_kind_check CHECK (kind IN ('payment', 'settlement', 'adjustment')),
	CONSTRAINT settlement_entries_amount_check CHECK (
		(kind IN ('payment', 'settlement') AND amount > 0) OR (kind = 'adjustment' AND amount <> 0)
	),
	CONSTRAINT settlement_entries_payment_refs_check CHECK (
		kind <> 'payment' OR (invoice_id IS NOT NULL AND provider_reference IS NOT NULL AND char_length(btrim(provider_reference)) > 0)
	),
	CONSTRAINT fk_settlement_entries_org FOREIGN KEY (beneficiary_organization_id) REFERENCES organizations(id),
	CONSTRAINT fk_settlement_entries_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);
-- Idempotensi pemasukan: satu entri payment per referensi provider.
CREATE UNIQUE INDEX IF NOT EXISTS idx_settlement_entries_payment_reference
	ON settlement_entries(kind, provider_reference) WHERE kind = 'payment';
CREATE INDEX IF NOT EXISTS idx_settlement_entries_beneficiary
	ON settlement_entries(beneficiary_organization_id, occurred_at);
