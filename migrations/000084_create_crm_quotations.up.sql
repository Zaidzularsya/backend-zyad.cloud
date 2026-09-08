CREATE TABLE IF NOT EXISTS crm_quotations (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	deal_id uuid,
	contact_id uuid,
	company_id uuid,
	quotation_number varchar(50) NOT NULL,
	status varchar(20) NOT NULL DEFAULT 'draft',
	valid_until date,
	subtotal numeric(18, 2) NOT NULL DEFAULT 0,
	discount_total numeric(18, 2) NOT NULL DEFAULT 0,
	tax_total numeric(18, 2) NOT NULL DEFAULT 0,
	grand_total numeric(18, 2) NOT NULL DEFAULT 0,
	currency char(3) NOT NULL DEFAULT 'IDR',
	notes text,
	sent_at timestamp without time zone,
	approved_at timestamp without time zone,
	rejected_at timestamp without time zone,
	created_by uuid,
	updated_by uuid,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	deleted_at timestamp without time zone,
	CONSTRAINT crm_quotations_organization_id_id_unique UNIQUE (organization_id, id),
	CONSTRAINT crm_quotations_status_check CHECK (
		status IN ('draft', 'sent', 'approved', 'rejected', 'expired')
	),
	CONSTRAINT fk_crm_quotations_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_quotations_deal
		FOREIGN KEY (organization_id, deal_id)
		REFERENCES crm_deals(organization_id, id)
		ON DELETE SET NULL,
	CONSTRAINT fk_crm_quotations_contact
		FOREIGN KEY (organization_id, contact_id)
		REFERENCES crm_contacts(organization_id, id)
		ON DELETE SET NULL,
	CONSTRAINT fk_crm_quotations_company
		FOREIGN KEY (organization_id, company_id)
		REFERENCES crm_companies(organization_id, id)
		ON DELETE SET NULL,
	CONSTRAINT fk_crm_quotations_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_crm_quotations_updated_by
		FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_crm_quotations_organization_number_active_unique
	ON crm_quotations(organization_id, quotation_number)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_quotations_organization_status
	ON crm_quotations(organization_id, status)
	WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_crm_quotations_deleted_at
	ON crm_quotations(deleted_at);

SELECT apply_organization_rls('crm_quotations'::regclass);

CREATE TABLE IF NOT EXISTS crm_quotation_items (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	quotation_id uuid NOT NULL,
	description varchar(500) NOT NULL,
	quantity numeric(18, 2) NOT NULL DEFAULT 1,
	unit_price numeric(18, 2) NOT NULL DEFAULT 0,
	discount_percent numeric(5, 2),
	line_total numeric(18, 2) NOT NULL DEFAULT 0,
	position integer NOT NULL DEFAULT 0,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT crm_quotation_items_quantity_check CHECK (quantity >= 0),
	CONSTRAINT crm_quotation_items_unit_price_check CHECK (unit_price >= 0),
	CONSTRAINT crm_quotation_items_discount_percent_check CHECK (
		discount_percent IS NULL OR (discount_percent >= 0 AND discount_percent <= 100)
	),
	CONSTRAINT fk_crm_quotation_items_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_crm_quotation_items_quotation
		FOREIGN KEY (organization_id, quotation_id)
		REFERENCES crm_quotations(organization_id, id)
		ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_crm_quotation_items_quotation
	ON crm_quotation_items(organization_id, quotation_id, position);

SELECT apply_organization_rls('crm_quotation_items'::regclass);
