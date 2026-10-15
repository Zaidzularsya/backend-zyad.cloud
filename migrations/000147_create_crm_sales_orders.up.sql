-- Sales Order (CRM): lahir otomatis dari quotation yang di-approve, dikonfirmasi sales, lalu
-- ditagih lewat modul receivable. receivable_account_id / initial_invoice_id / contract_id
-- sengaja tanpa FK lintas modul (modul receivable tidak dikenal skema CRM).

CREATE TABLE IF NOT EXISTS crm_sales_orders (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	so_number varchar(50) NOT NULL,
	quotation_id uuid NOT NULL,
	deal_id uuid NULL,
	contact_id uuid NULL,
	company_id uuid NULL,
	status varchar(20) NOT NULL DEFAULT 'draft',
	billing_status varchar(20) NOT NULL DEFAULT 'none',
	billing_error text NOT NULL DEFAULT '',
	start_date date NULL,
	bill_to_name varchar(200) NOT NULL DEFAULT '',
	bill_to_company varchar(200) NOT NULL DEFAULT '',
	bill_to_email varchar(255) NOT NULL DEFAULT '',
	bill_to_phone varchar(50) NOT NULL DEFAULT '',
	bill_to_address text NOT NULL DEFAULT '',
	channels text[] NOT NULL DEFAULT '{}',
	pic_user_id uuid NULL,
	currency char(3) NOT NULL DEFAULT 'IDR',
	subtotal numeric(18, 2) NOT NULL DEFAULT 0,
	tax_total numeric(18, 2) NOT NULL DEFAULT 0,
	grand_total numeric(18, 2) NOT NULL DEFAULT 0,
	first_invoice_total numeric(18, 2) NOT NULL DEFAULT 0,
	recurring_totals jsonb NOT NULL DEFAULT '{}'::jsonb,
	receivable_account_id uuid NULL,
	initial_invoice_id uuid NULL,
	contract_id uuid NULL,
	confirmed_at timestamp without time zone NULL,
	confirmed_by uuid NULL,
	cancelled_at timestamp without time zone NULL,
	created_by uuid NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT crm_sales_orders_org_id_unique UNIQUE (organization_id, id),
	CONSTRAINT crm_sales_orders_status_check CHECK (status IN ('draft', 'confirmed', 'completed', 'cancelled')),
	CONSTRAINT crm_sales_orders_billing_status_check CHECK (billing_status IN ('none', 'pending', 'done', 'failed')),
	CONSTRAINT fk_crm_sales_orders_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_sales_orders_quotation FOREIGN KEY (organization_id, quotation_id)
		REFERENCES crm_quotations(organization_id, id),
	CONSTRAINT fk_crm_sales_orders_deal FOREIGN KEY (organization_id, deal_id)
		REFERENCES crm_deals(organization_id, id),
	CONSTRAINT fk_crm_sales_orders_contact FOREIGN KEY (organization_id, contact_id)
		REFERENCES crm_contacts(organization_id, id),
	CONSTRAINT fk_crm_sales_orders_company FOREIGN KEY (organization_id, company_id)
		REFERENCES crm_companies(organization_id, id),
	CONSTRAINT fk_crm_sales_orders_pic FOREIGN KEY (pic_user_id) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_sales_orders_confirmed_by FOREIGN KEY (confirmed_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_sales_orders_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_crm_sales_orders_quotation_unique
	ON crm_sales_orders(organization_id, quotation_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_crm_sales_orders_number_unique
	ON crm_sales_orders(organization_id, so_number);
CREATE INDEX IF NOT EXISTS idx_crm_sales_orders_deal
	ON crm_sales_orders(organization_id, deal_id);
CREATE INDEX IF NOT EXISTS idx_crm_sales_orders_status
	ON crm_sales_orders(organization_id, status, created_at DESC);
SELECT apply_organization_rls('crm_sales_orders'::regclass);

CREATE TABLE IF NOT EXISTS crm_sales_order_items (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	sales_order_id uuid NOT NULL,
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
	charge_type varchar(20) NOT NULL DEFAULT 'one_time',
	billing_frequency varchar(20) NULL,
	payment_timing varchar(20) NOT NULL DEFAULT 'prepaid',
	delivery_status varchar(20) NOT NULL DEFAULT 'not_applicable',
	delivered_at date NULL,
	delivered_by uuid NULL,
	delivery_note varchar(500) NOT NULL DEFAULT '',
	invoice_id uuid NULL,
	position integer NOT NULL DEFAULT 0,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT crm_sales_order_items_charge_type_check CHECK (charge_type IN ('one_time', 'recurring')),
	CONSTRAINT crm_sales_order_items_frequency_check CHECK (billing_frequency IS NULL OR billing_frequency IN ('daily','weekly','monthly','quarterly','semiannual','annual')),
	CONSTRAINT crm_sales_order_items_frequency_required CHECK ((charge_type = 'recurring') = (billing_frequency IS NOT NULL)),
	CONSTRAINT crm_sales_order_items_timing_check CHECK (payment_timing IN ('prepaid', 'postpaid')),
	CONSTRAINT crm_sales_order_items_delivery_check CHECK (delivery_status IN ('not_applicable', 'pending', 'delivered')),
	CONSTRAINT fk_crm_sales_order_items_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_sales_order_items_order FOREIGN KEY (organization_id, sales_order_id)
		REFERENCES crm_sales_orders(organization_id, id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_sales_order_items_delivered_by FOREIGN KEY (delivered_by) REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_crm_sales_order_items_order
	ON crm_sales_order_items(organization_id, sales_order_id, position);
SELECT apply_organization_rls('crm_sales_order_items'::regclass);

ALTER TABLE crm_document_counters DROP CONSTRAINT IF EXISTS crm_document_counters_document_type_check;
ALTER TABLE crm_document_counters ADD CONSTRAINT crm_document_counters_document_type_check CHECK (
	document_type IN ('quotation', 'invoice', 'sales_order'));

ALTER TABLE crm_activities DROP CONSTRAINT IF EXISTS crm_activities_type_check;
ALTER TABLE crm_activities ADD CONSTRAINT crm_activities_type_check CHECK (
	type IN ('call', 'email', 'meeting', 'task', 'note', 'whatsapp', 'quotation_response', 'order'));

WITH so_permissions(permission_name, module, action, description) AS (
	VALUES
		('sales_order.read', 'crm', 'read', 'View sales orders'),
		('sales_order.manage', 'crm', 'manage', 'Edit and cancel draft sales orders'),
		('sales_order.confirm', 'crm', 'confirm', 'Confirm sales orders and delivery')
),
upserted_permissions AS (
	INSERT INTO permissions (permission_name, module, action, name, slug, description, created_at, updated_at)
	SELECT permission_name, module, action, permission_name, permission_name, description, now(), now()
	FROM so_permissions
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

INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT roles.id, permissions.id, 'organization', now()
FROM roles
CROSS JOIN permissions
WHERE (roles.role_name = 'member' OR roles.slug = 'member') AND permissions.permission_name = 'sales_order.read'
ON CONFLICT (role_id, permission_id)
DO UPDATE SET scope = EXCLUDED.scope, granted_at = EXCLUDED.granted_at;

INSERT INTO notification_templates (
	code, name, description, channel, locale, subject_template, body_template,
	available_variables, sample_payload, status, is_system, is_active, version, created_at, updated_at
) VALUES (
	'crm.sales_order_created', 'CRM Sales Order Created', 'Sent to the sales order PIC when an approved quotation produces a draft sales order', 'email', 'id-ID',
	'SO {{so_number}} siap dikonfirmasi',
	'Hi {{user_name}}, penawaran {{quotation_number}} disetujui. Lengkapi tanggal mulai dan data penagihan lalu konfirmasi: {{so_url}}',
	'[
		{"key":"app_name","description":"Application name","required":true,"example":"Zyad Cloud"},
		{"key":"user_name","description":"Recipient user name","required":true,"example":"Sari"},
		{"key":"so_number","description":"Sales order number","required":true,"example":"SO-2026-0001"},
		{"key":"quotation_number","description":"Source quotation number","required":true,"example":"QUO-2026-0001"},
		{"key":"so_url","description":"Sales order page URL","required":true,"example":"https://app.example.test/app/sales/orders/1"}
	]'::jsonb,
	'{"app_name":"Zyad Cloud","user_name":"Sari","so_number":"SO-2026-0001","quotation_number":"QUO-2026-0001","so_url":"https://app.example.test/app/sales/orders/1"}'::jsonb,
	'active', true, true, 1, now(), now()
)
ON CONFLICT (code, channel, locale, version)
DO UPDATE SET
	name = EXCLUDED.name, description = EXCLUDED.description, subject_template = EXCLUDED.subject_template,
	body_template = EXCLUDED.body_template, available_variables = EXCLUDED.available_variables,
	sample_payload = EXCLUDED.sample_payload, status = 'active', is_system = true, is_active = true, updated_at = now()
WHERE notification_templates.is_system = true;
