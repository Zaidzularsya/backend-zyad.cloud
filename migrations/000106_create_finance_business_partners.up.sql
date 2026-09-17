CREATE TABLE IF NOT EXISTS finance_business_partners (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	partner_type varchar(10) NOT NULL,
	code varchar(30) NOT NULL,
	name varchar(150) NOT NULL,
	tax_id varchar(30),
	address text,
	contact_info jsonb NOT NULL DEFAULT '{}'::jsonb,
	control_account_id uuid NOT NULL,
	is_active boolean NOT NULL DEFAULT true,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT finance_business_partners_type_check CHECK (partner_type IN ('customer', 'vendor')),
	CONSTRAINT finance_business_partners_name_not_blank_check CHECK (char_length(btrim(name)) > 0),
	CONSTRAINT finance_business_partners_contact_info_object_check CHECK (jsonb_typeof(contact_info) = 'object'),
	CONSTRAINT fk_finance_business_partners_control_account_id
		FOREIGN KEY (control_account_id) REFERENCES finance_accounts(id) ON DELETE RESTRICT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_finance_business_partners_code_active_unique
	ON finance_business_partners(code)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_business_partners_type_active
	ON finance_business_partners(partner_type, is_active)
	WHERE deleted_at IS NULL;
