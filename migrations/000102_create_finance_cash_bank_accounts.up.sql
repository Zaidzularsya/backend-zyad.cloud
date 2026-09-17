CREATE TABLE IF NOT EXISTS finance_cash_bank_accounts (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	account_id uuid NOT NULL,
	type varchar(10) NOT NULL,
	bank_name varchar(150),
	account_number varchar(50),
	account_holder_name varchar(150),
	currency varchar(3) NOT NULL DEFAULT 'IDR',
	is_active boolean NOT NULL DEFAULT true,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_cash_bank_accounts_type_check CHECK (type IN ('cash', 'bank')),
	CONSTRAINT finance_cash_bank_accounts_account_id_unique UNIQUE (account_id),
	CONSTRAINT fk_finance_cash_bank_accounts_account_id
		FOREIGN KEY (account_id) REFERENCES finance_accounts(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_finance_cash_bank_accounts_active
	ON finance_cash_bank_accounts(is_active);
