-- Jalur darurat, BUKAN rollback data: baris source 'contract'/'default'
-- dicabut (revoked) lalu source-nya diubah ke 'trial' supaya constraint lama
-- bisa dipulihkan. Riwayat pemberian dari contract tidak dipertahankan.
UPDATE organization_entitlements
SET status = 'revoked', source = 'trial', updated_at = now()
WHERE source IN ('contract', 'default');

ALTER TABLE organization_entitlements
	DROP CONSTRAINT IF EXISTS organization_entitlements_source_check;
ALTER TABLE organization_entitlements
	ADD CONSTRAINT organization_entitlements_source_check CHECK (
		source IN ('plan', 'addon', 'trial', 'platform_override')
	);

ALTER TABLE receivable_contract_items DROP COLUMN IF EXISTS features;
ALTER TABLE crm_sales_order_items DROP COLUMN IF EXISTS features;

DELETE FROM role_permissions
USING permissions
WHERE role_permissions.permission_id = permissions.id
	AND permissions.permission_name = 'company.link_workspace';
DELETE FROM permissions WHERE permission_name = 'company.link_workspace';
