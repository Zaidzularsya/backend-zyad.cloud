ALTER TABLE finance_journal_entries
	DROP CONSTRAINT finance_journal_entries_source_type_check;

ALTER TABLE finance_journal_entries
	ADD CONSTRAINT finance_journal_entries_source_type_check CHECK (
		source_type IN ('manual', 'opening_balance', 'depreciation', 'ar', 'ap', 'tax', 'cash_bank')
	);
