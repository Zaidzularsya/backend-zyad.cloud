DELETE FROM role_permissions
USING permissions
WHERE role_permissions.permission_id = permissions.id
	AND permissions.permission_name = 'payout_account.manage';
DELETE FROM permissions WHERE permission_name = 'payout_account.manage';
ALTER TABLE settlement_entries DROP CONSTRAINT IF EXISTS fk_settlement_entries_payout_account;
ALTER TABLE settlement_entries DROP COLUMN IF EXISTS payout_account_id;
DROP TABLE IF EXISTS organization_payout_accounts;
ALTER TABLE users DROP COLUMN IF EXISTS username_changed_at;
