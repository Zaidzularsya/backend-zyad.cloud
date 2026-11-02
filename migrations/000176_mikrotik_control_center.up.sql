-- Control center Mikrotik: mode isolir tingkat tenant, grace days, daftar akun pilot, dan masa berlaku pengecualian.
CREATE TABLE IF NOT EXISTS mikrotik_org_settings (
	organization_id uuid PRIMARY KEY,
	isolation_mode varchar(12) NOT NULL DEFAULT 'simulation',
	grace_days integer NOT NULL DEFAULT 0,
	pilot_verified_at timestamp without time zone,
	updated_by uuid,
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT mikrotik_org_settings_mode_check CHECK (isolation_mode IN ('simulation', 'pilot', 'active', 'paused')),
	CONSTRAINT mikrotik_org_settings_grace_check CHECK (grace_days BETWEEN 0 AND 60),
	CONSTRAINT fk_mikrotik_org_settings_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
);

SELECT apply_organization_rls('mikrotik_org_settings'::regclass);

CREATE TABLE IF NOT EXISTS mikrotik_pilot_accounts (
	organization_id uuid NOT NULL,
	account_id uuid NOT NULL,
	added_by uuid,
	added_at timestamp without time zone NOT NULL DEFAULT now(),
	test_started_at timestamp without time zone,
	test_until timestamp without time zone,
	PRIMARY KEY (organization_id, account_id),
	CONSTRAINT fk_mikrotik_pilot_accounts_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_mikrotik_pilot_accounts_account
		FOREIGN KEY (organization_id, account_id) REFERENCES receivable_accounts(organization_id, id) ON DELETE CASCADE
);

SELECT apply_organization_rls('mikrotik_pilot_accounts'::regclass);

ALTER TABLE mikrotik_customer_policies ADD COLUMN IF NOT EXISTS exempted_until date;
