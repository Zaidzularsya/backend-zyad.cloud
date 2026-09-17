CREATE TABLE IF NOT EXISTS finance_ar_ap_payments (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	partner_id uuid NOT NULL,
	payment_date date NOT NULL,
	amount numeric(18,2) NOT NULL,
	cash_bank_account_id uuid NOT NULL,
	journal_entry_id uuid NOT NULL,
	notes text,
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_ar_ap_payments_amount_check CHECK (amount > 0),
	CONSTRAINT fk_finance_ar_ap_payments_partner_id
		FOREIGN KEY (partner_id) REFERENCES finance_business_partners(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_ar_ap_payments_cash_bank_account_id
		FOREIGN KEY (cash_bank_account_id) REFERENCES finance_cash_bank_accounts(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_ar_ap_payments_journal_entry_id
		FOREIGN KEY (journal_entry_id) REFERENCES finance_journal_entries(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_ar_ap_payments_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_finance_ar_ap_payments_partner
	ON finance_ar_ap_payments(partner_id, payment_date);

CREATE TABLE IF NOT EXISTS finance_ar_ap_payment_allocations (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	payment_id uuid NOT NULL,
	ar_ap_transaction_id uuid NOT NULL,
	allocated_amount numeric(18,2) NOT NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_ar_ap_payment_allocations_amount_check CHECK (allocated_amount > 0),
	CONSTRAINT fk_finance_ar_ap_payment_allocations_payment_id
		FOREIGN KEY (payment_id) REFERENCES finance_ar_ap_payments(id) ON DELETE CASCADE,
	CONSTRAINT fk_finance_ar_ap_payment_allocations_transaction_id
		FOREIGN KEY (ar_ap_transaction_id) REFERENCES finance_ar_ap_transactions(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_finance_ar_ap_payment_allocations_transaction
	ON finance_ar_ap_payment_allocations(ar_ap_transaction_id);

CREATE INDEX IF NOT EXISTS idx_finance_ar_ap_payment_allocations_payment
	ON finance_ar_ap_payment_allocations(payment_id);
