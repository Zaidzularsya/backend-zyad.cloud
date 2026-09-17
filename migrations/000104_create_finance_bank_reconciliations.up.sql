CREATE TABLE IF NOT EXISTS finance_bank_reconciliations (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	cash_bank_account_id uuid NOT NULL,
	statement_date date NOT NULL,
	statement_ending_balance numeric(18,2) NOT NULL,
	book_ending_balance numeric(18,2) NOT NULL,
	status varchar(10) NOT NULL DEFAULT 'draft',
	notes text,
	completed_at timestamp without time zone,
	completed_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_bank_reconciliations_status_check CHECK (status IN ('draft', 'completed')),
	CONSTRAINT fk_finance_bank_reconciliations_cash_bank_account_id
		FOREIGN KEY (cash_bank_account_id) REFERENCES finance_cash_bank_accounts(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_bank_reconciliations_completed_by
		FOREIGN KEY (completed_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_finance_bank_reconciliations_account
	ON finance_bank_reconciliations(cash_bank_account_id, statement_date);
