-- Gelombang 2: progress of onboarding a customer from one PPP secret
-- (contact → deal → quotation → sales order → contract → paired). One row per
-- secret makes the onboarding idempotent; stage is the last completed step so
-- a retry continues from there. CRM/receivable ids carry no cross-module FK
-- (same convention as crm_sales_orders).
CREATE TABLE IF NOT EXISTS mikrotik_secret_onboardings (
	organization_id uuid NOT NULL,
	secret_id uuid NOT NULL,
	stage varchar(20) NOT NULL DEFAULT '',
	contact_id uuid NULL,
	deal_id uuid NULL,
	quotation_id uuid NULL,
	sales_order_id uuid NULL,
	contract_id uuid NULL,
	account_id uuid NULL,
	last_error text NOT NULL DEFAULT '',
	created_by uuid NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	PRIMARY KEY (organization_id, secret_id),
	CONSTRAINT mikrotik_secret_onboardings_stage_check CHECK (
		stage IN ('', 'contact', 'deal', 'quotation', 'sales_order', 'confirmed', 'paired')
	),
	CONSTRAINT fk_mikrotik_secret_onboardings_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_mikrotik_secret_onboardings_secret
		FOREIGN KEY (organization_id, secret_id) REFERENCES mikrotik_ppp_secrets(organization_id, id) ON DELETE CASCADE,
	CONSTRAINT fk_mikrotik_secret_onboardings_created_by
		FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

SELECT apply_organization_rls('mikrotik_secret_onboardings'::regclass);
