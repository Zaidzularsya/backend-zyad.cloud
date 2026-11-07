-- Usulan pairing secret PPPoE -> pelanggan (hasil perhitungan, bisa dibangun ulang) dan
-- grant mikrotik_task.read ke role member. mikrotik_task.retry sengaja TIDAK diberikan ke member.
CREATE TABLE IF NOT EXISTS mikrotik_secret_suggestions (
	organization_id uuid NOT NULL,
	secret_id uuid NOT NULL,
	account_id uuid,
	basis varchar(10) NOT NULL,
	candidates integer NOT NULL DEFAULT 1,
	computed_at timestamp without time zone NOT NULL DEFAULT now(),
	PRIMARY KEY (organization_id, secret_id),
	CONSTRAINT mikrotik_secret_suggestions_basis_check CHECK (basis IN ('phone', 'name', 'ambiguous')),
	CONSTRAINT mikrotik_secret_suggestions_candidates_check CHECK (candidates >= 1),
	CONSTRAINT mikrotik_secret_suggestions_ambiguous_check CHECK (
		(basis = 'ambiguous' AND account_id IS NULL AND candidates >= 2)
		OR (basis <> 'ambiguous' AND account_id IS NOT NULL)
	),
	CONSTRAINT fk_mikrotik_secret_suggestions_organization_id
		FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE,
	CONSTRAINT fk_mikrotik_secret_suggestions_secret
		FOREIGN KEY (organization_id, secret_id) REFERENCES mikrotik_ppp_secrets(organization_id, id) ON DELETE CASCADE,
	CONSTRAINT fk_mikrotik_secret_suggestions_account
		FOREIGN KEY (organization_id, account_id) REFERENCES receivable_accounts(organization_id, id)
);

SELECT apply_organization_rls('mikrotik_secret_suggestions'::regclass);

INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT roles.id, permissions.id, 'organization', now()
FROM roles
CROSS JOIN permissions
WHERE (roles.role_name = 'member' OR roles.slug = 'member') AND permissions.permission_name = 'mikrotik_task.read'
ON CONFLICT (role_id, permission_id)
DO UPDATE SET scope = EXCLUDED.scope, granted_at = EXCLUDED.granted_at;
