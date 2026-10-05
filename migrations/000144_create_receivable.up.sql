-- Modul receivable: penagihan tenant ke pelanggannya sendiri (BUKAN billing_invoices,
-- yaitu tagihan platform ke tenant). Semua tabel ber-scope organization_id + RLS.

CREATE TABLE IF NOT EXISTS receivable_settings (
	organization_id uuid PRIMARY KEY,
	invoice_lead_days integer NOT NULL DEFAULT 7,
	payment_terms_days integer NOT NULL DEFAULT 7,
	default_channels text[] NOT NULL DEFAULT '{email}',
	default_sender_user_id uuid NULL,
	updated_by uuid NULL,
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT receivable_settings_lead_days_check CHECK (invoice_lead_days BETWEEN 0 AND 60),
	CONSTRAINT receivable_settings_terms_check CHECK (payment_terms_days BETWEEN 0 AND 90),
	CONSTRAINT fk_receivable_settings_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_receivable_settings_sender FOREIGN KEY (default_sender_user_id) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_receivable_settings_updated_by FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);
SELECT apply_organization_rls('receivable_settings'::regclass);

CREATE TABLE IF NOT EXISTS receivable_document_counters (
	organization_id uuid NOT NULL,
	document_type varchar(20) NOT NULL,
	year integer NOT NULL,
	last_number integer NOT NULL DEFAULT 0,
	PRIMARY KEY (organization_id, document_type, year),
	CONSTRAINT receivable_document_counters_type_check CHECK (document_type IN ('invoice', 'contract')),
	CONSTRAINT fk_receivable_document_counters_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);
SELECT apply_organization_rls('receivable_document_counters'::regclass);

CREATE TABLE IF NOT EXISTS receivable_accounts (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	name varchar(200) NOT NULL,
	company_name varchar(200) NOT NULL DEFAULT '',
	email varchar(255) NOT NULL DEFAULT '',
	phone varchar(50) NOT NULL DEFAULT '',
	address text NOT NULL DEFAULT '',
	source_type varchar(30) NULL,
	source_id uuid NULL,
	created_by uuid NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone NULL,
	CONSTRAINT receivable_accounts_org_id_unique UNIQUE (organization_id, id),
	CONSTRAINT receivable_accounts_name_not_blank CHECK (char_length(btrim(name)) > 0),
	CONSTRAINT receivable_accounts_source_type_check CHECK (source_type IS NULL OR source_type IN ('crm_contact')),
	CONSTRAINT receivable_accounts_source_pair_check CHECK ((source_type IS NULL) = (source_id IS NULL)),
	CONSTRAINT fk_receivable_accounts_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_receivable_accounts_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_receivable_accounts_source_unique
	ON receivable_accounts(organization_id, source_type, source_id)
	WHERE source_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_receivable_accounts_org_name
	ON receivable_accounts(organization_id, lower(name)) WHERE deleted_at IS NULL;
SELECT apply_organization_rls('receivable_accounts'::regclass);

CREATE TABLE IF NOT EXISTS receivable_invoices (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	account_id uuid NOT NULL,
	invoice_number varchar(50) NULL, -- diisi saat terbit; NULL selama draft
	status varchar(20) NOT NULL DEFAULT 'draft',
	source_type varchar(20) NOT NULL DEFAULT 'manual',
	source_id uuid NULL,
	contract_id uuid NULL, -- FK ditambahkan S4
	idempotency_key varchar(100) NULL,
	period_start date NULL,
	period_end date NULL,
	issue_date date NULL,
	due_date date NULL,
	currency char(3) NOT NULL DEFAULT 'IDR',
	subtotal numeric(18, 2) NOT NULL DEFAULT 0,
	discount_total numeric(18, 2) NOT NULL DEFAULT 0,
	tax_total numeric(18, 2) NOT NULL DEFAULT 0,
	grand_total numeric(18, 2) NOT NULL DEFAULT 0,
	amount_paid numeric(18, 2) NOT NULL DEFAULT 0,
	channels text[] NOT NULL DEFAULT '{email}',
	pic_user_id uuid NULL,
	pdf_asset_id uuid NULL,
	notes text NOT NULL DEFAULT '',
	paid_at timestamp without time zone NULL,
	voided_at timestamp without time zone NULL,
	void_reason varchar(500) NOT NULL DEFAULT '',
	created_by uuid NULL,
	updated_by uuid NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT receivable_invoices_org_id_unique UNIQUE (organization_id, id),
	CONSTRAINT receivable_invoices_status_check CHECK (status IN ('draft', 'issued', 'paid', 'overdue', 'void')),
	CONSTRAINT receivable_invoices_source_type_check CHECK (source_type IN ('sales_order', 'contract', 'manual')),
	CONSTRAINT receivable_invoices_amount_paid_check CHECK (amount_paid >= 0 AND amount_paid <= grand_total),
	CONSTRAINT receivable_invoices_period_check CHECK (period_end IS NULL OR period_start IS NULL OR period_end >= period_start),
	CONSTRAINT fk_receivable_invoices_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_receivable_invoices_account FOREIGN KEY (organization_id, account_id)
		REFERENCES receivable_accounts(organization_id, id) ON DELETE RESTRICT,
	CONSTRAINT fk_receivable_invoices_pic FOREIGN KEY (pic_user_id) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_receivable_invoices_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_receivable_invoices_updated_by FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_receivable_invoices_number_unique
	ON receivable_invoices(organization_id, invoice_number) WHERE invoice_number IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_receivable_invoices_source_idempotency
	ON receivable_invoices(organization_id, source_type, source_id, idempotency_key)
	WHERE source_id IS NOT NULL AND idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_receivable_invoices_status_due
	ON receivable_invoices(organization_id, status, due_date);
CREATE INDEX IF NOT EXISTS idx_receivable_invoices_account
	ON receivable_invoices(organization_id, account_id);
SELECT apply_organization_rls('receivable_invoices'::regclass);

CREATE TABLE IF NOT EXISTS receivable_invoice_items (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	invoice_id uuid NOT NULL,
	description varchar(500) NOT NULL,
	quantity numeric(18, 2) NOT NULL DEFAULT 1,
	unit varchar(30) NOT NULL DEFAULT '',
	unit_price numeric(18, 2) NOT NULL DEFAULT 0,
	discount_percent numeric(5, 2) NULL,
	tax_percent numeric(5, 2) NOT NULL DEFAULT 0,
	tax_amount numeric(18, 2) NOT NULL DEFAULT 0,
	line_total numeric(18, 2) NOT NULL DEFAULT 0,
	product_id uuid NULL,
	sku varchar(64) NOT NULL DEFAULT '',
	contract_item_id uuid NULL,
	source_line_id uuid NULL,
	period_start date NULL,
	period_end date NULL,
	charge_type varchar(20) NOT NULL DEFAULT 'one_time',
	billing_frequency varchar(20) NULL,
	payment_timing varchar(20) NOT NULL DEFAULT 'prepaid',
	position integer NOT NULL DEFAULT 0,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT receivable_invoice_items_org_id_unique UNIQUE (organization_id, id),
	CONSTRAINT receivable_invoice_items_charge_type_check CHECK (charge_type IN ('one_time', 'recurring')),
	CONSTRAINT receivable_invoice_items_billing_frequency_check CHECK (billing_frequency IS NULL OR billing_frequency IN ('daily','weekly','monthly','quarterly','semiannual','annual')),
	CONSTRAINT receivable_invoice_items_frequency_required CHECK ((charge_type = 'recurring') = (billing_frequency IS NOT NULL)),
	CONSTRAINT receivable_invoice_items_payment_timing_check CHECK (payment_timing IN ('prepaid', 'postpaid')),
	CONSTRAINT fk_receivable_invoice_items_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_receivable_invoice_items_invoice FOREIGN KEY (organization_id, invoice_id)
		REFERENCES receivable_invoices(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_receivable_invoice_items_invoice
	ON receivable_invoice_items(organization_id, invoice_id, position);
CREATE UNIQUE INDEX IF NOT EXISTS idx_receivable_invoice_items_contract_period
	ON receivable_invoice_items(organization_id, contract_item_id, period_start) WHERE contract_item_id IS NOT NULL;
SELECT apply_organization_rls('receivable_invoice_items'::regclass);

CREATE TABLE IF NOT EXISTS receivable_invoice_sends (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	invoice_id uuid NOT NULL,
	channel varchar(20) NOT NULL,
	recipient varchar(255) NOT NULL DEFAULT '',
	status varchar(20) NOT NULL,
	error text NOT NULL DEFAULT '',
	external_message_id varchar(255) NOT NULL DEFAULT '',
	client_request_id varchar(100) NULL,
	trigger varchar(10) NOT NULL DEFAULT 'manual',
	sent_by uuid NULL,
	sent_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT receivable_invoice_sends_channel_check CHECK (channel IN ('email', 'whatsapp')),
	CONSTRAINT receivable_invoice_sends_status_check CHECK (status IN ('sent', 'failed')),
	CONSTRAINT receivable_invoice_sends_trigger_check CHECK (trigger IN ('auto', 'manual')),
	CONSTRAINT fk_receivable_invoice_sends_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_receivable_invoice_sends_invoice FOREIGN KEY (organization_id, invoice_id)
		REFERENCES receivable_invoices(organization_id, id) ON DELETE CASCADE,
	CONSTRAINT fk_receivable_invoice_sends_sent_by FOREIGN KEY (sent_by) REFERENCES users(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_receivable_invoice_sends_client_request
	ON receivable_invoice_sends(organization_id, invoice_id, client_request_id) WHERE client_request_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_receivable_invoice_sends_invoice
	ON receivable_invoice_sends(organization_id, invoice_id, sent_at DESC);
SELECT apply_organization_rls('receivable_invoice_sends'::regclass);

CREATE TABLE IF NOT EXISTS receivable_payments (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	invoice_id uuid NOT NULL,
	amount numeric(18, 2) NOT NULL,
	method varchar(20) NOT NULL,
	reference varchar(150) NULL,
	note varchar(500) NOT NULL DEFAULT '',
	provider_payload jsonb NULL,
	recorded_by uuid NULL,
	paid_at timestamp without time zone NOT NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT receivable_payments_amount_check CHECK (amount > 0),
	CONSTRAINT receivable_payments_method_check CHECK (method IN ('manual', 'doku')),
	CONSTRAINT fk_receivable_payments_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_receivable_payments_invoice FOREIGN KEY (organization_id, invoice_id)
		REFERENCES receivable_invoices(organization_id, id) ON DELETE RESTRICT,
	CONSTRAINT fk_receivable_payments_recorded_by FOREIGN KEY (recorded_by) REFERENCES users(id) ON DELETE SET NULL
);
-- Referensi unik global per metode: webhook provider yang sama tidak boleh dicatat dua kali.
CREATE UNIQUE INDEX IF NOT EXISTS idx_receivable_payments_method_reference
	ON receivable_payments(method, reference) WHERE reference IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_receivable_payments_invoice
	ON receivable_payments(organization_id, invoice_id, paid_at);
SELECT apply_organization_rls('receivable_payments'::regclass);

-- Feature & entitlement.
INSERT INTO product_features (feature_key, module, name, description, value_type, unit, reset_strategy, is_active, created_at, updated_at)
VALUES
	('receivable.enabled', 'receivable', 'Receivable module', 'Enable invoicing & recurring billing for tenant customers', 'boolean', NULL, 'never', true, now(), now()),
	('receivable.online_payment', 'receivable', 'Receivable online payment', 'Collect invoice payments online via the platform gateway', 'boolean', NULL, 'never', true, now(), now())
ON CONFLICT (feature_key)
DO UPDATE SET module = EXCLUDED.module, name = EXCLUDED.name, description = EXCLUDED.description,
	value_type = EXCLUDED.value_type, unit = EXCLUDED.unit, reset_strategy = EXCLUDED.reset_strategy,
	is_active = EXCLUDED.is_active, updated_at = now();

WITH plan_entitlements_seed(plan_code, feature_key, value_bool) AS (
	VALUES
		('free', 'receivable.enabled', false),
		('starter', 'receivable.enabled', true),
		('growth', 'receivable.enabled', true),
		('business', 'receivable.enabled', true),
		('enterprise', 'receivable.enabled', true),
		('free', 'receivable.online_payment', false),
		('starter', 'receivable.online_payment', false),
		('growth', 'receivable.online_payment', false),
		('business', 'receivable.online_payment', false),
		('enterprise', 'receivable.online_payment', false)
)
INSERT INTO product_plan_entitlements (plan_id, feature_id, value_bool, value_int, value_decimal, value_string, limits, created_at, updated_at)
SELECT plans.id, features.id, seed.value_bool, NULL, NULL, NULL, '{}'::jsonb, now(), now()
FROM plan_entitlements_seed seed
JOIN product_plans plans ON plans.code = seed.plan_code AND plans.deleted_at IS NULL
JOIN product_features features ON features.feature_key = seed.feature_key
ON CONFLICT (plan_id, feature_id)
DO UPDATE SET value_bool = EXCLUDED.value_bool, value_int = EXCLUDED.value_int, value_decimal = EXCLUDED.value_decimal,
	value_string = EXCLUDED.value_string, limits = EXCLUDED.limits, updated_at = now();

-- Permission: invoice.* pindah modul ke receivable; tambah invoice.void & receivable.settings.
UPDATE permissions SET module = 'receivable' WHERE permission_name LIKE 'invoice.%';

WITH receivable_permissions(permission_name, module, action, description) AS (
	VALUES
		('invoice.void', 'receivable', 'void', 'Void receivable invoices'),
		('receivable.settings', 'receivable', 'settings', 'Manage receivable billing settings')
),
upserted_permissions AS (
	INSERT INTO permissions (permission_name, module, action, name, slug, description, created_at, updated_at)
	SELECT permission_name, module, action, permission_name, permission_name, description, now(), now()
	FROM receivable_permissions
	ON CONFLICT (slug)
	DO UPDATE SET permission_name = EXCLUDED.permission_name, module = EXCLUDED.module, action = EXCLUDED.action,
		name = EXCLUDED.name, slug = EXCLUDED.slug, description = EXCLUDED.description, updated_at = now()
	RETURNING id
),
full_access_roles AS (
	SELECT id FROM roles
	WHERE role_name IN ('organization_owner', 'super_admin') OR slug IN ('organization_owner', 'super_admin')
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT full_access_roles.id, upserted_permissions.id, 'organization', now()
FROM full_access_roles CROSS JOIN upserted_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET scope = EXCLUDED.scope, granted_at = EXCLUDED.granted_at;

-- Notifikasi ke PIC/admin saat pengiriman invoice gagal.
INSERT INTO notification_templates (
	code, name, description, channel, locale, subject_template, body_template,
	available_variables, sample_payload, status, is_system, is_active, version, created_at, updated_at
) VALUES (
	'receivable.invoice_send_failed', 'Receivable Invoice Send Failed', 'Sent to the invoice PIC or billing admins when sending an invoice fails', 'email', 'id-ID',
	'Invoice {{invoice_number}} gagal dikirim',
	'Hi {{user_name}}, pengiriman invoice {{invoice_number}} ke {{recipient}} via {{channel}} gagal: {{reason}}. Buka {{invoice_url}} untuk mengirim ulang.',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"user_name","description":"Recipient user name","required":true,"example":"Sari"},
		{"key":"invoice_number","description":"Invoice number","required":true,"example":"INV-2026-0001"},
		{"key":"recipient","description":"Intended recipient (email or phone)","required":true,"example":"budi@example.test"},
		{"key":"channel","description":"Channel label (Email or WhatsApp)","required":true,"example":"Email"},
		{"key":"reason","description":"Failure reason (no provider internals)","required":true,"example":"Belum ada pengirim."},
		{"key":"invoice_url","description":"Invoice page URL","required":true,"example":"https://app.example.test/app/billing/invoices/1"}
	]'::jsonb,
	'{"app_name":"Zyad Cloud","user_name":"Sari","invoice_number":"INV-2026-0001","recipient":"budi@example.test","channel":"Email","reason":"Belum ada pengirim.","invoice_url":"https://app.example.test/app/billing/invoices/1"}'::jsonb,
	'active', true, true, 1, now(), now()
)
ON CONFLICT (code, channel, locale, version)
DO UPDATE SET
	name = EXCLUDED.name, description = EXCLUDED.description, subject_template = EXCLUDED.subject_template,
	body_template = EXCLUDED.body_template, available_variables = EXCLUDED.available_variables,
	sample_payload = EXCLUDED.sample_payload, status = 'active', is_system = true, is_active = true, updated_at = now()
WHERE notification_templates.is_system = true;
