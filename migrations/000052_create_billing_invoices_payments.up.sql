CREATE TABLE IF NOT EXISTS billing_invoices (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	subscription_id uuid,
	invoice_number varchar(80) NOT NULL,
	status varchar(30) NOT NULL,
	currency varchar(3) NOT NULL DEFAULT 'IDR',
	subtotal_amount numeric(14,2) NOT NULL DEFAULT 0,
	discount_amount numeric(14,2) NOT NULL DEFAULT 0,
	tax_amount numeric(14,2) NOT NULL DEFAULT 0,
	total_amount numeric(14,2) NOT NULL DEFAULT 0,
	due_date timestamp without time zone,
	paid_at timestamp without time zone,
	metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT billing_invoices_number_unique UNIQUE (invoice_number),
	CONSTRAINT billing_invoices_identity_unique UNIQUE (id, organization_id),
	CONSTRAINT billing_invoices_status_check CHECK (
		status IN ('draft', 'open', 'paid', 'void', 'expired', 'failed')
	),
	CONSTRAINT billing_invoices_currency_check CHECK (
		currency = upper(currency)
		AND currency ~ '^[A-Z]{3}$'
	),
	CONSTRAINT billing_invoices_amount_check CHECK (
		subtotal_amount >= 0
		AND discount_amount >= 0
		AND tax_amount >= 0
		AND total_amount >= 0
	),
	CONSTRAINT billing_invoices_paid_check CHECK (
		status <> 'paid'
		OR paid_at IS NOT NULL
	),
	CONSTRAINT billing_invoices_metadata_object_check CHECK (
		jsonb_typeof(metadata) = 'object'
	),
	CONSTRAINT fk_billing_invoices_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_billing_invoices_subscription_org
		FOREIGN KEY (subscription_id, organization_id)
		REFERENCES billing_subscriptions(id, organization_id)
		ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_billing_invoices_org_status_created
	ON billing_invoices(organization_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_billing_invoices_subscription
	ON billing_invoices(subscription_id)
	WHERE subscription_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_billing_invoices_due_date
	ON billing_invoices(due_date)
	WHERE due_date IS NOT NULL;

CREATE TABLE IF NOT EXISTS billing_invoice_items (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	invoice_id uuid NOT NULL,
	item_type varchar(30) NOT NULL,
	description text NOT NULL,
	quantity numeric(14,2) NOT NULL DEFAULT 1,
	unit_amount numeric(14,2) NOT NULL DEFAULT 0,
	total_amount numeric(14,2) NOT NULL DEFAULT 0,
	metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT billing_invoice_items_type_check CHECK (
		item_type IN ('subscription', 'addon', 'adjustment', 'tax', 'discount')
	),
	CONSTRAINT billing_invoice_items_description_not_blank_check CHECK (
		char_length(btrim(description)) > 0
	),
	CONSTRAINT billing_invoice_items_amount_check CHECK (
		quantity > 0
		AND unit_amount >= 0
		AND total_amount >= 0
	),
	CONSTRAINT billing_invoice_items_metadata_object_check CHECK (
		jsonb_typeof(metadata) = 'object'
	),
	CONSTRAINT fk_billing_invoice_items_invoice_id
		FOREIGN KEY (invoice_id) REFERENCES billing_invoices(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_billing_invoice_items_invoice
	ON billing_invoice_items(invoice_id);

CREATE TABLE IF NOT EXISTS billing_payments (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	invoice_id uuid NOT NULL,
	organization_id uuid NOT NULL,
	provider varchar(30) NOT NULL,
	provider_reference varchar(150),
	payment_method varchar(80),
	status varchar(30) NOT NULL,
	amount numeric(14,2) NOT NULL,
	currency varchar(3) NOT NULL DEFAULT 'IDR',
	paid_at timestamp without time zone,
	raw_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT billing_payments_status_check CHECK (
		status IN ('pending', 'paid', 'failed', 'expired', 'refunded')
	),
	CONSTRAINT billing_payments_provider_check CHECK (
		provider IN ('manual', 'xendit', 'midtrans')
	),
	CONSTRAINT billing_payments_currency_check CHECK (
		currency = upper(currency)
		AND currency ~ '^[A-Z]{3}$'
	),
	CONSTRAINT billing_payments_amount_check CHECK (amount >= 0),
	CONSTRAINT billing_payments_paid_check CHECK (
		status <> 'paid'
		OR paid_at IS NOT NULL
	),
	CONSTRAINT billing_payments_raw_payload_object_check CHECK (
		jsonb_typeof(raw_payload) = 'object'
	),
	CONSTRAINT fk_billing_payments_invoice_org
		FOREIGN KEY (invoice_id, organization_id)
		REFERENCES billing_invoices(id, organization_id)
		ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_billing_payments_provider_reference_unique
	ON billing_payments(provider, provider_reference)
	WHERE provider_reference IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_billing_payments_org_status_created
	ON billing_payments(organization_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_billing_payments_invoice
	ON billing_payments(invoice_id);
