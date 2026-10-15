-- Contract tagihan berulang (modul receivable). Dibuat saat Sales Order dikonfirmasi (S4);
-- billing run berkala menagih item per periode (S5).

CREATE TABLE IF NOT EXISTS receivable_contracts (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	account_id uuid NOT NULL,
	contract_number varchar(50) NOT NULL,
	status varchar(20) NOT NULL DEFAULT 'active',
	source_type varchar(20) NOT NULL DEFAULT 'manual',
	source_id uuid NULL,
	currency char(3) NOT NULL DEFAULT 'IDR',
	start_date date NOT NULL,
	end_date date NULL,
	channels text[] NOT NULL DEFAULT '{email}',
	pic_user_id uuid NULL,
	notes text NOT NULL DEFAULT '',
	end_reason varchar(500) NOT NULL DEFAULT '',
	ended_at timestamp without time zone NULL,
	created_by uuid NULL,
	updated_by uuid NULL,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT receivable_contracts_org_id_unique UNIQUE (organization_id, id),
	CONSTRAINT receivable_contracts_status_check CHECK (status IN ('active', 'ended', 'cancelled')),
	CONSTRAINT receivable_contracts_source_type_check CHECK (source_type IN ('sales_order', 'manual')),
	CONSTRAINT receivable_contracts_end_date_check CHECK (end_date IS NULL OR end_date >= start_date),
	CONSTRAINT fk_receivable_contracts_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_receivable_contracts_account FOREIGN KEY (organization_id, account_id)
		REFERENCES receivable_accounts(organization_id, id) ON DELETE RESTRICT,
	CONSTRAINT fk_receivable_contracts_pic FOREIGN KEY (pic_user_id) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_receivable_contracts_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_receivable_contracts_updated_by FOREIGN KEY (updated_by) REFERENCES users(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_receivable_contracts_number_unique
	ON receivable_contracts(organization_id, contract_number);
CREATE UNIQUE INDEX IF NOT EXISTS idx_receivable_contracts_source_unique
	ON receivable_contracts(organization_id, source_type, source_id) WHERE source_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_receivable_contracts_status
	ON receivable_contracts(organization_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_receivable_contracts_account
	ON receivable_contracts(organization_id, account_id);
SELECT apply_organization_rls('receivable_contracts'::regclass);

CREATE TABLE IF NOT EXISTS receivable_contract_items (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	contract_id uuid NOT NULL,
	description varchar(500) NOT NULL,
	quantity numeric(18, 2) NOT NULL DEFAULT 1,
	unit varchar(30) NOT NULL DEFAULT '',
	unit_price numeric(18, 2) NOT NULL DEFAULT 0,
	discount_percent numeric(5, 2) NULL,
	tax_percent numeric(5, 2) NOT NULL DEFAULT 0,
	product_id uuid NULL,
	sku varchar(64) NOT NULL DEFAULT '',
	source_line_id uuid NULL,
	billing_frequency varchar(20) NOT NULL,
	payment_timing varchar(20) NOT NULL,
	period_index integer NOT NULL DEFAULT 0,
	next_period_start date NOT NULL,
	next_period_end date NOT NULL,
	position integer NOT NULL DEFAULT 0,
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT receivable_contract_items_org_id_unique UNIQUE (organization_id, id),
	CONSTRAINT receivable_contract_items_frequency_check CHECK (billing_frequency IN ('daily','weekly','monthly','quarterly','semiannual','annual')),
	CONSTRAINT receivable_contract_items_timing_check CHECK (payment_timing IN ('prepaid', 'postpaid')),
	CONSTRAINT receivable_contract_items_period_index_check CHECK (period_index >= 0),
	CONSTRAINT receivable_contract_items_period_check CHECK (next_period_end >= next_period_start),
	CONSTRAINT fk_receivable_contract_items_org FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_receivable_contract_items_contract FOREIGN KEY (organization_id, contract_id)
		REFERENCES receivable_contracts(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_receivable_contract_items_contract
	ON receivable_contract_items(organization_id, contract_id, position);
CREATE INDEX IF NOT EXISTS idx_receivable_contract_items_next
	ON receivable_contract_items(organization_id, next_period_start);
SELECT apply_organization_rls('receivable_contract_items'::regclass);

ALTER TABLE receivable_invoices
	ADD CONSTRAINT fk_receivable_invoices_contract FOREIGN KEY (organization_id, contract_id)
	REFERENCES receivable_contracts(organization_id, id) ON DELETE RESTRICT;
ALTER TABLE receivable_invoice_items
	ADD CONSTRAINT fk_receivable_invoice_items_contract_item FOREIGN KEY (organization_id, contract_item_id)
	REFERENCES receivable_contract_items(organization_id, id) ON DELETE RESTRICT;

WITH contract_permissions(permission_name, module, action, description) AS (
	VALUES
		('contract.read', 'receivable', 'read', 'View receivable contracts'),
		('contract.manage', 'receivable', 'manage', 'Manage receivable contracts')
),
upserted_permissions AS (
	INSERT INTO permissions (permission_name, module, action, name, slug, description, created_at, updated_at)
	SELECT permission_name, module, action, permission_name, permission_name, description, now(), now()
	FROM contract_permissions
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
