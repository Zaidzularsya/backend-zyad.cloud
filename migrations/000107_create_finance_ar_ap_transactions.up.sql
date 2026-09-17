CREATE TABLE IF NOT EXISTS finance_ar_ap_transactions (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	partner_id uuid NOT NULL,
	transaction_type varchar(10) NOT NULL,
	transaction_date date NOT NULL,
	due_date date NOT NULL,
	reference_number varchar(150),
	amount numeric(18,2) NOT NULL,
	contra_account_id uuid NOT NULL,
	description text,
	status varchar(20) NOT NULL DEFAULT 'open',
	journal_entry_id uuid NOT NULL,
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_ar_ap_transactions_type_check CHECK (transaction_type IN ('receivable', 'payable')),
	CONSTRAINT finance_ar_ap_transactions_amount_check CHECK (amount > 0),
	CONSTRAINT finance_ar_ap_transactions_status_check CHECK (status IN ('open', 'partially_paid', 'paid', 'void')),
	CONSTRAINT finance_ar_ap_transactions_due_after_date_check CHECK (due_date >= transaction_date),
	CONSTRAINT fk_finance_ar_ap_transactions_partner_id
		FOREIGN KEY (partner_id) REFERENCES finance_business_partners(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_ar_ap_transactions_contra_account_id
		FOREIGN KEY (contra_account_id) REFERENCES finance_accounts(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_ar_ap_transactions_journal_entry_id
		FOREIGN KEY (journal_entry_id) REFERENCES finance_journal_entries(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_ar_ap_transactions_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_finance_ar_ap_transactions_partner
	ON finance_ar_ap_transactions(partner_id, status);

CREATE INDEX IF NOT EXISTS idx_finance_ar_ap_transactions_due_date
	ON finance_ar_ap_transactions(transaction_type, due_date)
	WHERE status IN ('open', 'partially_paid');
