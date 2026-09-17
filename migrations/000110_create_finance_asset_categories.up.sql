CREATE TABLE IF NOT EXISTS finance_asset_categories (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	code varchar(30) NOT NULL,
	name varchar(150) NOT NULL,
	asset_account_id uuid NOT NULL,
	accumulated_depreciation_account_id uuid NOT NULL,
	depreciation_expense_account_id uuid NOT NULL,
	default_useful_life_months int,
	is_active boolean NOT NULL DEFAULT true,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_asset_categories_name_not_blank_check CHECK (char_length(btrim(name)) > 0),
	CONSTRAINT finance_asset_categories_useful_life_check CHECK (default_useful_life_months IS NULL OR default_useful_life_months > 0),
	CONSTRAINT fk_finance_asset_categories_asset_account_id
		FOREIGN KEY (asset_account_id) REFERENCES finance_accounts(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_asset_categories_accum_depr_account_id
		FOREIGN KEY (accumulated_depreciation_account_id) REFERENCES finance_accounts(id) ON DELETE RESTRICT,
	CONSTRAINT fk_finance_asset_categories_depr_expense_account_id
		FOREIGN KEY (depreciation_expense_account_id) REFERENCES finance_accounts(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_asset_categories_code_unique
	ON finance_asset_categories(code);
