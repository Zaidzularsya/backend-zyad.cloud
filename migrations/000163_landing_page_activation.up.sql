-- Tenant products 2C: page activation (reseller sells one landing page to a client).
-- active_until ends the paid publish period; NULL = never activated (behaves as before).
-- Timestamps are UTC (DB session pinned to UTC); WIB is display-only (businesstime).
ALTER TABLE landing_pages
	ADD COLUMN IF NOT EXISTS active_until timestamp without time zone NULL;

CREATE TABLE IF NOT EXISTS landing_page_activations (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	page_id uuid NOT NULL,
	invoice_id uuid NULL,
	buyer_name varchar(200) NOT NULL,
	buyer_contact varchar(100) NOT NULL,
	buyer_email varchar(255) NULL,
	price numeric(20,2) NOT NULL,
	duration_value integer NOT NULL,
	duration_unit varchar(10) NOT NULL,
	preview_token_hash text NOT NULL,
	invoice_due_at timestamp without time zone NOT NULL,
	status varchar(10) NOT NULL DEFAULT 'open',
	paid_at timestamp without time zone NULL,
	created_by uuid NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT landing_page_activations_org_id_unique UNIQUE (organization_id, id),
	CONSTRAINT landing_page_activations_invoice_id_unique UNIQUE (invoice_id),
	CONSTRAINT landing_page_activations_preview_token_hash_unique UNIQUE (preview_token_hash),
	CONSTRAINT landing_page_activations_buyer_name_not_blank_check CHECK (char_length(btrim(buyer_name)) > 0),
	CONSTRAINT landing_page_activations_buyer_contact_not_blank_check CHECK (char_length(btrim(buyer_contact)) > 0),
	CONSTRAINT landing_page_activations_price_check CHECK (price >= 0),
	CONSTRAINT landing_page_activations_duration_value_check CHECK (duration_value > 0),
	CONSTRAINT landing_page_activations_duration_unit_check CHECK (duration_unit IN ('day', 'week', 'month')),
	CONSTRAINT landing_page_activations_status_check CHECK (status IN ('open', 'paid', 'void')),
	CONSTRAINT landing_page_activations_paid_at_check CHECK ((status = 'paid') = (paid_at IS NOT NULL)),
	CONSTRAINT fk_landing_page_activations_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	-- Composite FK keeps the activation in the page's organization.
	CONSTRAINT fk_landing_page_activations_page
		FOREIGN KEY (organization_id, page_id)
		REFERENCES landing_pages(organization_id, id) ON DELETE CASCADE,
	-- The activation invoice is owned by the same (reseller) organization; NULL until Task 13 links it.
	CONSTRAINT fk_landing_page_activations_invoice
		FOREIGN KEY (organization_id, invoice_id)
		REFERENCES receivable_invoices(organization_id, id),
	CONSTRAINT fk_landing_page_activations_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

-- At most one open activation per page (spec 6b.14).
CREATE UNIQUE INDEX IF NOT EXISTS idx_landing_page_activations_one_open_per_page
	ON landing_page_activations(organization_id, page_id)
	WHERE status = 'open';

CREATE INDEX IF NOT EXISTS idx_landing_page_activations_org_page_created
	ON landing_page_activations(organization_id, page_id, created_at DESC);

SELECT apply_organization_rls('landing_page_activations'::regclass);
