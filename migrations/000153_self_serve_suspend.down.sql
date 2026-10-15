DROP INDEX IF EXISTS idx_receivable_invoices_overdue_due;

DELETE FROM notification_templates
WHERE code IN ('self_serve.workspace_suspended', 'self_serve.workspace_reactivated') AND is_system = true;
