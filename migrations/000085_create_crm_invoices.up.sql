-- crm_invoices is tenant-to-their-own-customer invoicing — NOT the same as
-- billing_invoices (platform-to-tenant SaaS subscription billing). See
-- docs/reference-crm.md "Naming Conflict" for why the two must never be
-- confused, and why this table is deliberately not named just "invoices".
CREATE TABLE IF NOT EXISTS crm_invoices (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	quotation_id uuid,
	deal_id uuid,
	contact_id uuid,
	company_id uuid,
	invoice_number varchar(50) NOT NULL,
	status varchar(20) NOT NULL DEFAULT 'draft',
	issue_date date,
	due_date date,
	subtotal numeric(18, 2) NOT NULL DEFAULT 0,
	tax_total numeric(18, 2) NOT NULL DEFAULT 0,
	grand_total numeric(18, 2) NOT NULL DEFAULT 0,
	amount_paid numeric(18, 2) NOT NULL DEFAULT 0,
	paid_at timestamp without time zone,
	currency char(3) NOT NULL DEFAULT 'IDR',
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT crm_invoices_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT crm_invoices_status_check CHECK (
		status IN ('draft', 'sent', 'paid', 'overdue', 'cancelled')
	),
	CONSTRAINT crm_invoices_amount_paid_check CHECK (amount_paid >= 0),
	CONSTRAINT fk_crm_invoices_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_invoices_quotation
		FOREIGN KEY (organization_id, quotation_id)
		REFERENCES crm_quotations(organization_id, id)
		ON DELETE SET NULL,
	CONSTRAINT fk_crm_invoices_deal
		FOREIGN KEY (organization_id, deal_id)
		REFERENCES crm_deals(organization_id, id)
		ON DELETE SET NULL,
	CONSTRAINT fk_crm_invoices_contact
		FOREIGN KEY (organization_id, contact_id)
		REFERENCES crm_contacts(organization_id, id)
		ON DELETE SET NULL,
	CONSTRAINT fk_crm_invoices_company
		FOREIGN KEY (organization_id, company_id)
		REFERENCES crm_companies(organization_id, id)
		ON DELETE SET NULL,
	CONSTRAINT fk_crm_invoices_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_invoices_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_crm_invoices_organization_number_active_unique
	ON crm_invoices(organization_id, invoice_number)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_invoices_organization_status
	ON crm_invoices(organization_id, status)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_invoices_deleted_at
	ON crm_invoices(deleted_at);

SELECT apply_organization_rls('crm_invoices'::regclass);

CREATE TABLE IF NOT EXISTS crm_invoice_items (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	invoice_id uuid NOT NULL,
	description varchar(500) NOT NULL,
	quantity numeric(18, 2) NOT NULL DEFAULT 1,
	unit_price numeric(18, 2) NOT NULL DEFAULT 0,
	discount_percent numeric(5, 2),
	line_total numeric(18, 2) NOT NULL DEFAULT 0,
	position integer NOT NULL DEFAULT 0,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT crm_invoice_items_quantity_check CHECK (quantity >= 0),
	CONSTRAINT crm_invoice_items_unit_price_check CHECK (unit_price >= 0),
	CONSTRAINT crm_invoice_items_discount_percent_check CHECK (
		discount_percent IS NULL OR (discount_percent >= 0 AND discount_percent <= 100)
	),
	CONSTRAINT fk_crm_invoice_items_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_invoice_items_invoice
		FOREIGN KEY (organization_id, invoice_id)
		REFERENCES crm_invoices(organization_id, id)
		ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_crm_invoice_items_invoice
	ON crm_invoice_items(organization_id, invoice_id, position);

SELECT apply_organization_rls('crm_invoice_items'::regclass);
