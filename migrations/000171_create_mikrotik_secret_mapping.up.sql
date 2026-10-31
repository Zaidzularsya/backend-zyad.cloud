-- Fase 2 isolir PPPoE: cache PPP secret hasil impor router, pemetaan ke pelanggan
-- (receivable_accounts), pengecualian isolir per pelanggan, dan penanda produk "layanan PPPoE".
CREATE TABLE IF NOT EXISTS mikrotik_ppp_secrets (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	router_id uuid NOT NULL,
	name varchar(100) NOT NULL,
	profile varchar(100) NOT NULL DEFAULT '',
	original_profile varchar(100),
	needs_original_profile boolean NOT NULL DEFAULT false,
	disabled boolean NOT NULL DEFAULT false,
	comment text NOT NULL DEFAULT '',
	present boolean NOT NULL DEFAULT true,
	last_seen_at timestamp without time zone,
	isolation_state varchar(10) NOT NULL DEFAULT 'normal',
	account_id uuid,
	paired_by uuid,
	paired_at timestamp without time zone,
	imported_at timestamp without time zone NOT NULL DEFAULT now(),
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT mikrotik_ppp_secrets_router_name_unique UNIQUE (router_id, name),
	CONSTRAINT mikrotik_ppp_secrets_name_not_blank_check CHECK (char_length(btrim(name)) > 0),
	CONSTRAINT mikrotik_ppp_secrets_isolation_state_check CHECK (isolation_state IN ('normal', 'isolated')),
	CONSTRAINT fk_mikrotik_ppp_secrets_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_mikrotik_ppp_secrets_router
		FOREIGN KEY (organization_id, router_id) REFERENCES mikrotik_routers(organization_id, id) ON DELETE CASCADE,
	CONSTRAINT fk_mikrotik_ppp_secrets_account
		FOREIGN KEY (account_id) REFERENCES receivable_accounts(id) ON DELETE SET NULL,
	CONSTRAINT fk_mikrotik_ppp_secrets_paired_by
		FOREIGN KEY (paired_by) REFERENCES users(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_mikrotik_ppp_secrets_org_account
	ON mikrotik_ppp_secrets(organization_id, account_id);

SELECT apply_organization_rls('mikrotik_ppp_secrets'::regclass);

CREATE TABLE IF NOT EXISTS mikrotik_customer_policies (
	organization_id uuid NOT NULL,
	account_id uuid NOT NULL,
	exempt boolean NOT NULL DEFAULT false,
	reason text NOT NULL DEFAULT '',
	set_by uuid,
	set_at timestamp without time zone NOT NULL DEFAULT now(),
	PRIMARY KEY (organization_id, account_id),
	CONSTRAINT fk_mikrotik_customer_policies_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_mikrotik_customer_policies_account
		FOREIGN KEY (account_id) REFERENCES receivable_accounts(id) ON DELETE CASCADE,
	CONSTRAINT fk_mikrotik_customer_policies_set_by
		FOREIGN KEY (set_by) REFERENCES users(id) ON DELETE SET NULL
);

SELECT apply_organization_rls('mikrotik_customer_policies'::regclass);

CREATE TABLE IF NOT EXISTS mikrotik_product_triggers (
	organization_id uuid NOT NULL,
	product_id uuid NOT NULL,
	enabled boolean NOT NULL DEFAULT false,
	effective_from date NOT NULL,
	set_by uuid,
	set_at timestamp without time zone NOT NULL DEFAULT now(),
	PRIMARY KEY (organization_id, product_id),
	CONSTRAINT fk_mikrotik_product_triggers_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_mikrotik_product_triggers_set_by
		FOREIGN KEY (set_by) REFERENCES users(id) ON DELETE SET NULL
);

SELECT apply_organization_rls('mikrotik_product_triggers'::regclass);
