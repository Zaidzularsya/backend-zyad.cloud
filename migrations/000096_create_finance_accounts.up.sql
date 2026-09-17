CREATE TABLE IF NOT EXISTS finance_accounts (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	account_code varchar(20) NOT NULL,
	account_name varchar(150) NOT NULL,
	account_category_id uuid,
	parent_account_id uuid,
	is_header boolean NOT NULL DEFAULT false,
	normal_balance varchar(10) NOT NULL,
	is_active boolean NOT NULL DEFAULT true,
	opening_balance numeric(18,2) NOT NULL DEFAULT 0,
	opening_balance_date date,
	description text,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT finance_accounts_normal_balance_check CHECK (
		normal_balance IN ('debit', 'credit')
	),
	CONSTRAINT finance_accounts_leaf_category_check CHECK (
		is_header = true OR account_category_id IS NOT NULL
	),
	CONSTRAINT finance_accounts_name_not_blank_check CHECK (
		char_length(btrim(account_name)) > 0
	),
	CONSTRAINT fk_finance_accounts_account_category_id
		FOREIGN KEY (account_category_id) REFERENCES finance_account_categories(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_accounts_parent_account_id
		FOREIGN KEY (parent_account_id) REFERENCES finance_accounts(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_accounts_code_active_unique
	ON finance_accounts(account_code)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_accounts_category
	ON finance_accounts(account_category_id)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_accounts_parent
	ON finance_accounts(parent_account_id)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_accounts_deleted_at
	ON finance_accounts(deleted_at);
