CREATE TABLE IF NOT EXISTS finance_account_types (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	code varchar(20) NOT NULL,
	name varchar(100) NOT NULL,
	normal_balance varchar(10) NOT NULL,
	financial_statement varchar(20) NOT NULL,
	sort_order integer NOT NULL DEFAULT 0,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_account_types_code_unique UNIQUE (code),
	CONSTRAINT finance_account_types_code_check CHECK (
		code IN ('asset', 'liability', 'equity', 'revenue', 'expense')
	),
	CONSTRAINT finance_account_types_normal_balance_check CHECK (
		normal_balance IN ('debit', 'credit')
	),
	CONSTRAINT finance_account_types_financial_statement_check CHECK (
		financial_statement IN ('balance_sheet', 'profit_loss')
	)
);

CREATE TABLE IF NOT EXISTS finance_account_categories (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	account_type_id uuid NOT NULL,
	code varchar(30) NOT NULL,
	name varchar(100) NOT NULL,
	report_section varchar(30) NOT NULL,
	sort_order integer NOT NULL DEFAULT 0,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_account_categories_code_unique UNIQUE (code),
	CONSTRAINT finance_account_categories_report_section_check CHECK (
		report_section IN (
			'current_asset', 'fixed_asset', 'other_asset',
			'current_liability', 'long_term_liability',
			'equity',
			'revenue', 'other_income',
			'cogs', 'operating_expense', 'other_expense'
		)
	),
	CONSTRAINT fk_finance_account_categories_account_type_id
		FOREIGN KEY (account_type_id) REFERENCES finance_account_types(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_finance_account_categories_account_type
	ON finance_account_categories(account_type_id);

WITH types(code, name, normal_balance, financial_statement, sort_order) AS (
	VALUES
		('asset', 'Aset', 'debit', 'balance_sheet', 1),
		('liability', 'Kewajiban', 'credit', 'balance_sheet', 2),
		('equity', 'Ekuitas', 'credit', 'balance_sheet', 3),
		('revenue', 'Pendapatan', 'credit', 'profit_loss', 4),
		('expense', 'Beban', 'debit', 'profit_loss', 5)
)
INSERT INTO finance_account_types (code, name, normal_balance, financial_statement, sort_order)
SELECT code, name, normal_balance, financial_statement, sort_order FROM types
ON CONFLICT (code) DO NOTHING;

WITH categories(type_code, code, name, report_section, sort_order) AS (
	VALUES
		('asset', 'current_asset', 'Aset Lancar', 'current_asset', 1),
		('asset', 'fixed_asset', 'Aset Tetap', 'fixed_asset', 2),
		('asset', 'other_asset', 'Aset Lainnya', 'other_asset', 3),
		('liability', 'current_liability', 'Kewajiban Jangka Pendek', 'current_liability', 1),
		('liability', 'long_term_liability', 'Kewajiban Jangka Panjang', 'long_term_liability', 2),
		('equity', 'equity', 'Ekuitas', 'equity', 1),
		('revenue', 'revenue', 'Pendapatan Usaha', 'revenue', 1),
		('revenue', 'other_income', 'Pendapatan Lain-lain', 'other_income', 2),
		('expense', 'cogs', 'Beban Pokok', 'cogs', 1),
		('expense', 'operating_expense', 'Beban Operasional', 'operating_expense', 2),
		('expense', 'other_expense', 'Beban Lain-lain', 'other_expense', 3)
)
INSERT INTO finance_account_categories (account_type_id, code, name, report_section, sort_order)
SELECT finance_account_types.id, categories.code, categories.name, categories.report_section, categories.sort_order
FROM categories
JOIN finance_account_types ON finance_account_types.code = categories.type_code
ON CONFLICT (code) DO NOTHING;
