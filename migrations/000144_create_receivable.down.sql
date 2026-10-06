DELETE FROM notification_templates WHERE code = 'receivable.invoice_send_failed' AND is_system = true;

DELETE FROM role_permissions
USING permissions
WHERE role_permissions.permission_id = permissions.id
	AND permissions.permission_name IN ('invoice.void', 'receivable.settings');
DELETE FROM permissions WHERE permission_name IN ('invoice.void', 'receivable.settings');
UPDATE permissions SET module = 'crm' WHERE permission_name LIKE 'invoice.%';

DELETE FROM product_plan_entitlements
WHERE feature_id IN (SELECT id FROM product_features WHERE feature_key IN ('receivable.enabled', 'receivable.online_payment'));
DELETE FROM product_features WHERE feature_key IN ('receivable.enabled', 'receivable.online_payment');

DROP TABLE IF EXISTS receivable_payments;
DROP TABLE IF EXISTS receivable_invoice_sends;
DROP TABLE IF EXISTS receivable_invoice_items;
DROP TABLE IF EXISTS receivable_invoices;
DROP TABLE IF EXISTS receivable_accounts;
DROP TABLE IF EXISTS receivable_document_counters;
DROP TABLE IF EXISTS receivable_settings;
