CREATE TABLE IF NOT EXISTS finance_fixed_assets (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	asset_category_id uuid NOT NULL,
	asset_code varchar(30) NOT NULL,
	asset_name varchar(150) NOT NULL,
	acquisition_date date NOT NULL,
	acquisition_cost numeric(18,2) NOT NULL,
	salvage_value numeric(18,2) NOT NULL DEFAULT 0,
	useful_life_months int NOT NULL,
	description text,
	contra_account_id uuid NOT NULL,
	acquisition_journal_entry_id uuid NOT NULL,
	status varchar(20) NOT NULL DEFAULT 'active',
	created_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_fixed_assets_name_not_blank_check CHECK (char_length(btrim(asset_name)) > 0),
	CONSTRAINT finance_fixed_assets_cost_check CHECK (acquisition_cost > 0),
	CONSTRAINT finance_fixed_assets_salvage_check CHECK (salvage_value >= 0 AND salvage_value < acquisition_cost),
	CONSTRAINT finance_fixed_assets_useful_life_check CHECK (useful_life_months > 0),
	CONSTRAINT finance_fixed_assets_status_check CHECK (status IN ('active', 'fully_depreciated')),
	CONSTRAINT fk_finance_fixed_assets_category_id
		FOREIGN KEY (asset_category_id) REFERENCES finance_asset_categories(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_fixed_assets_contra_account_id
		FOREIGN KEY (contra_account_id) REFERENCES finance_accounts(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_fixed_assets_acquisition_journal_entry_id
		FOREIGN KEY (acquisition_journal_entry_id) REFERENCES finance_journal_entries(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_fixed_assets_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_fixed_assets_code_unique
	ON finance_fixed_assets(asset_code);

CREATE INDEX IF NOT EXISTS idx_finance_fixed_assets_category
	ON finance_fixed_assets(asset_category_id);

CREATE INDEX IF NOT EXISTS idx_finance_fixed_assets_status
	ON finance_fixed_assets(status);
