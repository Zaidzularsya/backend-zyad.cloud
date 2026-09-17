CREATE SEQUENCE IF NOT EXISTS finance_journal_entry_number_seq;

CREATE TABLE IF NOT EXISTS finance_journal_entries (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	entry_number varchar(40) NOT NULL,
	entry_date date NOT NULL,
	fiscal_period_id uuid NOT NULL,
	source_type varchar(20) NOT NULL DEFAULT 'manual',
	source_id uuid,
	reference varchar(150),
	description text,
	status varchar(10) NOT NULL DEFAULT 'draft',
	posted_at timestamp without time zone,
	posted_by uuid,
	reversed_by_entry_id uuid,
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_journal_entries_entry_number_unique UNIQUE (entry_number),
	CONSTRAINT finance_journal_entries_source_type_check CHECK (
		source_type IN ('manual', 'opening_balance', 'depreciation', 'ar', 'ap', 'tax')
	),
	CONSTRAINT finance_journal_entries_status_check CHECK (
		status IN ('draft', 'posted', 'reversed')
	),
	CONSTRAINT fk_finance_journal_entries_fiscal_period_id
		FOREIGN KEY (fiscal_period_id) REFERENCES finance_fiscal_periods(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_journal_entries_posted_by
		FOREIGN KEY (posted_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_finance_journal_entries_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_finance_journal_entries_reversed_by_entry_id
		FOREIGN KEY (reversed_by_entry_id) REFERENCES finance_journal_entries(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_finance_journal_entries_entry_date
	ON finance_journal_entries(entry_date);

CREATE INDEX IF NOT EXISTS idx_finance_journal_entries_fiscal_period
	ON finance_journal_entries(fiscal_period_id);

CREATE INDEX IF NOT EXISTS idx_finance_journal_entries_source
	ON finance_journal_entries(source_type, source_id);

CREATE INDEX IF NOT EXISTS idx_finance_journal_entries_status
	ON finance_journal_entries(status);

CREATE TABLE IF NOT EXISTS finance_journal_lines (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	journal_entry_id uuid NOT NULL,
	line_number integer NOT NULL,
	account_id uuid NOT NULL,
	debit numeric(18,2) NOT NULL DEFAULT 0,
	credit numeric(18,2) NOT NULL DEFAULT 0,
	description text,
	subledger_type varchar(20) NOT NULL DEFAULT 'none',
	subledger_id uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_journal_lines_line_unique UNIQUE (journal_entry_id, line_number),
	CONSTRAINT finance_journal_lines_debit_nonneg_check CHECK (debit >= 0),
	CONSTRAINT finance_journal_lines_credit_nonneg_check CHECK (credit >= 0),
	CONSTRAINT finance_journal_lines_single_side_check CHECK (NOT (debit > 0 AND credit > 0)),
	CONSTRAINT finance_journal_lines_nonzero_check CHECK (debit + credit > 0),
	CONSTRAINT finance_journal_lines_subledger_type_check CHECK (
		subledger_type IN ('none', 'customer', 'vendor', 'asset', 'tax_code')
	),
	CONSTRAINT fk_finance_journal_lines_journal_entry_id
		FOREIGN KEY (journal_entry_id) REFERENCES finance_journal_entries(id) ON DELETE CASCADE,
	CONSTRAINT fk_finance_journal_lines_account_id
		FOREIGN KEY (account_id) REFERENCES finance_accounts(id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_finance_journal_lines_journal_entry
	ON finance_journal_lines(journal_entry_id);

CREATE INDEX IF NOT EXISTS idx_finance_journal_lines_account
	ON finance_journal_lines(account_id);

CREATE INDEX IF NOT EXISTS idx_finance_journal_lines_subledger
	ON finance_journal_lines(subledger_type, subledger_id);
