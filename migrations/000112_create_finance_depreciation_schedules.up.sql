CREATE TABLE IF NOT EXISTS finance_depreciation_schedules (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	fixed_asset_id uuid NOT NULL,
	sequence_number int NOT NULL,
	period_date date NOT NULL,
	depreciation_amount numeric(18,2) NOT NULL,
	status varchar(20) NOT NULL DEFAULT 'pending',
	journal_entry_id uuid,
	posted_at timestamp without time zone,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_depreciation_schedules_sequence_check CHECK (sequence_number > 0),
	CONSTRAINT finance_depreciation_schedules_amount_check CHECK (depreciation_amount >= 0),
	CONSTRAINT finance_depreciation_schedules_status_check CHECK (status IN ('pending', 'posted')),
	CONSTRAINT finance_depreciation_schedules_posted_journal_check CHECK (
		(status = 'posted' AND journal_entry_id IS NOT NULL AND posted_at IS NOT NULL)
		OR (status = 'pending' AND journal_entry_id IS NULL AND posted_at IS NULL)
	),
	CONSTRAINT fk_finance_depreciation_schedules_asset_id
		FOREIGN KEY (fixed_asset_id) REFERENCES finance_fixed_assets(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_depreciation_schedules_journal_entry_id
		FOREIGN KEY (journal_entry_id) REFERENCES finance_journal_entries(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_depreciation_schedules_asset_sequence_unique
	ON finance_depreciation_schedules(fixed_asset_id, sequence_number);

CREATE INDEX IF NOT EXISTS idx_finance_depreciation_schedules_pending_period
	ON finance_depreciation_schedules(period_date)
	WHERE status = 'pending';
