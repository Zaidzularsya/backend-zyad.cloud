-- Account & Profile: cooldown ganti username, rekening pencairan (payout) organisasi,
-- dan tautan settlement ke rekening yang dipakai.

-- Cooldown username (30 hari). NULL untuk user lama = penggantian pertama langsung boleh.
ALTER TABLE users ADD COLUMN IF NOT EXISTS username_changed_at timestamp without time zone NULL;

-- Rekening tujuan pencairan saldo settlement milik organisasi. Isi baris tidak pernah
-- di-update; perubahan rekening selalu berupa baris baru sehingga riwayat lengkap.
-- Data pembukuan platform yang dibaca lintas tenant oleh super admin: sengaja TANPA
-- apply_organization_rls (preseden settlement_entries, 000165). Isolasi dilakukan di
-- service layer lewat filter organization_id dari konteks tenant.
-- FK organisasi tanpa ON DELETE CASCADE: catatan uang tidak boleh ikut terhapus.
CREATE TABLE IF NOT EXISTS organization_payout_accounts (
	id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	organization_id uuid NOT NULL,
	bank_code varchar(20) NOT NULL,
	bank_name varchar(100) NOT NULL,
	account_number varchar(34) NOT NULL,
	account_holder_name varchar(150) NOT NULL,
	status varchar(20) NOT NULL,
	submitted_by uuid NULL,
	submitted_at timestamp without time zone NOT NULL,
	reviewed_by uuid NULL,
	reviewed_at timestamp without time zone NULL,
	review_note text NOT NULL DEFAULT '',
	created_at timestamp without time zone NOT NULL DEFAULT now(),
	updated_at timestamp without time zone NOT NULL DEFAULT now(),
	CONSTRAINT organization_payout_accounts_status_check CHECK (
		status IN ('pending', 'verified', 'rejected', 'cancelled', 'superseded')
	),
	CONSTRAINT fk_payout_accounts_org FOREIGN KEY (organization_id) REFERENCES organizations(id),
	CONSTRAINT fk_payout_accounts_submitted_by FOREIGN KEY (submitted_by) REFERENCES users(id) ON DELETE SET NULL,
	CONSTRAINT fk_payout_accounts_reviewed_by FOREIGN KEY (reviewed_by) REFERENCES users(id) ON DELETE SET NULL
);
-- Maksimal satu pengajuan pending dan satu rekening verified per organisasi.
CREATE UNIQUE INDEX IF NOT EXISTS idx_payout_accounts_one_pending
	ON organization_payout_accounts(organization_id) WHERE status = 'pending';
CREATE UNIQUE INDEX IF NOT EXISTS idx_payout_accounts_one_verified
	ON organization_payout_accounts(organization_id) WHERE status = 'verified';
-- Antrean verifikasi super admin.
CREATE INDEX IF NOT EXISTS idx_payout_accounts_status_submitted
	ON organization_payout_accounts(status, submitted_at);

-- Diisi untuk settlement baru; entri lama tetap NULL. Tanpa ON DELETE (rekening tidak dihapus).
ALTER TABLE settlement_entries ADD COLUMN IF NOT EXISTS payout_account_id uuid NULL;
ALTER TABLE settlement_entries
	ADD CONSTRAINT fk_settlement_entries_payout_account
	FOREIGN KEY (payout_account_id) REFERENCES organization_payout_accounts(id);

-- payout_account.manage: owner organisasi mengajukan/membatalkan rekening pencairan miliknya.
-- Super admin memverifikasi lewat settlement.manage yang sudah ada.
WITH new_permission AS (
	INSERT INTO permissions (
		permission_name, module, action, name, slug, description, created_at, updated_at
	)
	VALUES (
		'payout_account.manage', 'payout_account', 'manage', 'payout_account.manage',
		'payout_account.manage', 'Submit and cancel the own organization payout bank account', now(), now()
	)
	ON CONFLICT (slug)
	DO UPDATE SET
		permission_name = EXCLUDED.permission_name,
		module = EXCLUDED.module,
		action = EXCLUDED.action,
		name = EXCLUDED.name,
		description = EXCLUDED.description,
		updated_at = now()
	RETURNING id
),
target_roles AS (
	SELECT id
	FROM roles
	WHERE role_name IN ('organization_owner')
		OR slug IN ('organization_owner')
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT target_roles.id, new_permission.id, 'organization', now()
FROM target_roles
CROSS JOIN new_permission
ON CONFLICT (role_id, permission_id)
DO UPDATE SET scope = EXCLUDED.scope, granted_at = EXCLUDED.granted_at;
