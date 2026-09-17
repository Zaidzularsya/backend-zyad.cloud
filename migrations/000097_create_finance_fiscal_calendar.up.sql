CREATE TABLE IF NOT EXISTS finance_fiscal_years (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	year integer NOT NULL,
	start_date date NOT NULL,
	end_date date NOT NULL,
	status varchar(10) NOT NULL DEFAULT 'open',
	closed_at timestamp without time zone,
	closed_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_fiscal_years_year_unique UNIQUE (year),
	CONSTRAINT finance_fiscal_years_status_check CHECK (status IN ('open', 'closed')),
	CONSTRAINT finance_fiscal_years_date_range_check CHECK (end_date > start_date),
	CONSTRAINT fk_finance_fiscal_years_closed_by
		FOREIGN KEY (closed_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS finance_fiscal_periods (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	fiscal_year_id uuid NOT NULL,
	period_number integer NOT NULL,
	start_date date NOT NULL,
	end_date date NOT NULL,
	status varchar(10) NOT NULL DEFAULT 'open',
	closed_at timestamp without time zone,
	closed_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_fiscal_periods_number_unique UNIQUE (fiscal_year_id, period_number),
	CONSTRAINT finance_fiscal_periods_number_check CHECK (period_number BETWEEN 1 AND 12),
	CONSTRAINT finance_fiscal_periods_status_check CHECK (status IN ('open', 'closed')),
	CONSTRAINT finance_fiscal_periods_date_range_check CHECK (end_date > start_date),
	CONSTRAINT fk_finance_fiscal_periods_fiscal_year_id
		FOREIGN KEY (fiscal_year_id) REFERENCES finance_fiscal_years(id) ON DELETE CASCADE,
	CONSTRAINT fk_finance_fiscal_periods_closed_by
		FOREIGN KEY (closed_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_finance_fiscal_periods_fiscal_year
	ON finance_fiscal_periods(fiscal_year_id);

CREATE INDEX IF NOT EXISTS idx_finance_fiscal_periods_date_range
	ON finance_fiscal_periods(start_date, end_date);
