CREATE TABLE IF NOT EXISTS finance_tax_transactions (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	tax_type_id uuid NOT NULL,
	transaction_date date NOT NULL,
	reference_number varchar(150),
	amount numeric(18,2) NOT NULL,
	direction varchar(10) NOT NULL,
	tax_account_id uuid NOT NULL,
	contra_account_id uuid NOT NULL,
	description text,
	journal_entry_id uuid NOT NULL,
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_tax_transactions_amount_check CHECK (amount > 0),
	CONSTRAINT finance_tax_transactions_direction_check CHECK (direction IN ('increase', 'decrease')),
	CONSTRAINT fk_finance_tax_transactions_tax_type_id
		FOREIGN KEY (tax_type_id) REFERENCES finance_tax_types(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_tax_transactions_tax_account_id
		FOREIGN KEY (tax_account_id) REFERENCES finance_accounts(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_tax_transactions_contra_account_id
		FOREIGN KEY (contra_account_id) REFERENCES finance_accounts(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_tax_transactions_journal_entry_id
		FOREIGN KEY (journal_entry_id) REFERENCES finance_journal_entries(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_tax_transactions_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_finance_tax_transactions_type_date
	ON finance_tax_transactions(tax_type_id, transaction_date);
