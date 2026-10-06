DELETE FROM notification_templates WHERE code = 'crm.sales_order_created' AND is_system = true;
DELETE FROM role_permissions USING permissions
	WHERE role_permissions.permission_id = permissions.id AND permissions.permission_name LIKE 'sales_order.%';
DELETE FROM permissions WHERE permission_name LIKE 'sales_order.%';
DELETE FROM crm_activities WHERE type = 'order';
ALTER TABLE crm_activities DROP CONSTRAINT IF EXISTS crm_activities_type_check;
ALTER TABLE crm_activities ADD CONSTRAINT crm_activities_type_check CHECK (
	type IN ('call', 'email', 'meeting', 'task', 'note', 'whatsapp', 'quotation_response'));
DELETE FROM crm_document_counters WHERE document_type = 'sales_order';
ALTER TABLE crm_document_counters DROP CONSTRAINT IF EXISTS crm_document_counters_document_type_check;
ALTER TABLE crm_document_counters ADD CONSTRAINT crm_document_counters_document_type_check CHECK (
	document_type IN ('quotation', 'invoice'));
DROP TABLE IF EXISTS crm_sales_order_items;
DROP TABLE IF EXISTS crm_sales_orders;
