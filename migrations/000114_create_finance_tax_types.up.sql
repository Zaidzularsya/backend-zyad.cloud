CREATE TABLE IF NOT EXISTS finance_tax_types (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	code varchar(30) NOT NULL,
	name varchar(150) NOT NULL,
	category varchar(20) NOT NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT finance_tax_types_code_unique UNIQUE (code),
	CONSTRAINT finance_tax_types_category_check CHECK (
		category IN ('output_vat', 'input_vat', 'withholding', 'corporate_income')
	)
);

WITH types(code, name, category) AS (
	VALUES
		('PPN_KELUARAN', 'PPN Keluaran', 'output_vat'),
		('PPN_MASUKAN', 'PPN Masukan', 'input_vat'),
		('PPH21', 'PPh Pasal 21', 'withholding'),
		('PPH23', 'PPh Pasal 23', 'withholding'),
		('PPH_BADAN', 'PPh Badan', 'corporate_income')
)
INSERT INTO finance_tax_types (code, name, category)
SELECT code, name, category FROM types
ON CONFLICT (code) DO NOTHING;
