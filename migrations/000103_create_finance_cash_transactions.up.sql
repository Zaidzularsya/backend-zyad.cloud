CREATE TABLE IF NOT EXISTS finance_cash_transactions (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	cash_bank_account_id uuid NOT NULL,
	transaction_date date NOT NULL,
	transaction_type varchar(20) NOT NULL,
	amount numeric(18,2) NOT NULL,
	counter_cash_bank_account_id uuid,
	contra_account_id uuid,
	reference varchar(150),
	description text,
	journal_entry_id uuid NOT NULL,
	reconciled_at timestamp without time zone,
	reconciled_by uuid,
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_cash_transactions_type_check CHECK (
		transaction_type IN ('cash_in', 'cash_out', 'transfer')
	),
	CONSTRAINT finance_cash_transactions_amount_check CHECK (amount > 0),
	CONSTRAINT finance_cash_transactions_counterparty_check CHECK (
		(transaction_type = 'transfer' AND counter_cash_bank_account_id IS NOT NULL AND contra_account_id IS NULL)
		OR (transaction_type <> 'transfer' AND contra_account_id IS NOT NULL AND counter_cash_bank_account_id IS NULL)
	),
	CONSTRAINT fk_finance_cash_transactions_cash_bank_account_id
		FOREIGN KEY (cash_bank_account_id) REFERENCES finance_cash_bank_accounts(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_cash_transactions_counter_cash_bank_account_id
		FOREIGN KEY (counter_cash_bank_account_id) REFERENCES finance_cash_bank_accounts(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_cash_transactions_contra_account_id
		FOREIGN KEY (contra_account_id) REFERENCES finance_accounts(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_cash_transactions_journal_entry_id
		FOREIGN KEY (journal_entry_id) REFERENCES finance_journal_entries(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_cash_transactions_reconciled_by
		FOREIGN KEY (reconciled_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_finance_cash_transactions_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_finance_cash_transactions_account_date
	ON finance_cash_transactions(cash_bank_account_id, transaction_date);

CREATE INDEX IF NOT EXISTS idx_finance_cash_transactions_journal_entry
	ON finance_cash_transactions(journal_entry_id);

CREATE INDEX IF NOT EXISTS idx_finance_cash_transactions_reconciled
	ON finance_cash_transactions(cash_bank_account_id, reconciled_at);
