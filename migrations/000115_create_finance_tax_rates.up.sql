CREATE TABLE IF NOT EXISTS finance_tax_rates (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	tax_type_id uuid NOT NULL,
	rate_percent numeric(5,2) NOT NULL,
	effective_date date NOT NULL,
	end_date date,
	notes text,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_tax_rates_rate_check CHECK (rate_percent >= 0),
	CONSTRAINT finance_tax_rates_end_after_effective_check CHECK (end_date IS NULL OR end_date >= effective_date),
	CONSTRAINT fk_finance_tax_rates_tax_type_id
		FOREIGN KEY (tax_type_id) REFERENCES finance_tax_types(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_tax_rates_type_effective_unique
	ON finance_tax_rates(tax_type_id, effective_date);

WITH seed(code, rate_percent, effective_date, notes) AS (
	VALUES
		('PPN_KELUARAN', 11.00, DATE '2022-04-01', 'Tarif PPN 11% (UU HPP)'),
		('PPN_MASUKAN', 11.00, DATE '2022-04-01', 'Tarif PPN 11% (UU HPP)'),
		('PPH23', 2.00, DATE '2009-01-01', 'Tarif umum PPh 23 atas jasa')
)
INSERT INTO finance_tax_rates (tax_type_id, rate_percent, effective_date, notes)
SELECT tt.id, seed.rate_percent, seed.effective_date, seed.notes
FROM seed
JOIN finance_tax_types tt ON tt.code = seed.code
ON CONFLICT (tax_type_id, effective_date) DO NOTHING;
