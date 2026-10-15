ALTER TABLE receivable_invoice_items DROP CONSTRAINT IF EXISTS fk_receivable_invoice_items_contract_item;
ALTER TABLE receivable_invoices DROP CONSTRAINT IF EXISTS fk_receivable_invoices_contract;
DROP TABLE IF EXISTS receivable_contract_items;
DROP TABLE IF EXISTS receivable_contracts;
DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE slug IN ('contract.read', 'contract.manage'));
DELETE FROM permissions WHERE slug IN ('contract.read', 'contract.manage');
