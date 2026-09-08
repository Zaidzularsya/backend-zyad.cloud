-- Seeds permissions for the Fase 4 CRM resources (Quotation, Invoice CRM) —
-- matching what internal/modules/crm actually enforces today. Deferred from
-- SKILLS.md's matrix: view_price/view_amount/approve_discount field-level
-- masking (no granular price/amount hiding implemented yet — everyone with
-- read access sees full figures). See docs/reference-crm.md "Fase
-- Implementasi" and migrations/000078_...permissions.up.sql for the no-
-- super_admin-grant deviation this migration repeats.
WITH crm_permissions(permission_name, module, action, description) AS (
	VALUES
		('quotation.read', 'crm', 'read', 'Read CRM quotations'),
		('quotation.create', 'crm', 'create', 'Create CRM quotations'),
		('quotation.update', 'crm', 'update', 'Update CRM quotations'),
		('quotation.delete', 'crm', 'delete', 'Delete CRM quotations'),
		('quotation.send', 'crm', 'send', 'Send CRM quotations to customers'),
		('quotation.approve', 'crm', 'approve', 'Approve CRM quotations'),
		('quotation.reject', 'crm', 'reject', 'Reject CRM quotations'),
		('invoice.read', 'crm', 'read', 'Read CRM invoices'),
		('invoice.create', 'crm', 'create', 'Create CRM invoices'),
		('invoice.update', 'crm', 'update', 'Update CRM invoices'),
		('invoice.delete', 'crm', 'delete', 'Delete CRM invoices'),
		('invoice.send', 'crm', 'send', 'Send CRM invoices to customers'),
		('invoice.mark_paid', 'crm', 'mark_paid', 'Mark CRM invoices as paid'),
		('invoice.cancel', 'crm', 'cancel', 'Cancel CRM invoices')
),
upserted_permissions AS (
	INSERT INTO permissions (
		permission_name,
		module,
		action,
		name,
		slug,
		description,
		created_at,
		updated_at
	)
	SELECT
		permission_name,
		module,
		action,
		permission_name,
		permission_name,
		description,
		now(),
		now()
	FROM crm_permissions
	ON CONFLICT (slug)
	DO UPDATE SET
		permission_name = EXCLUDED.permission_name,
		module = EXCLUDED.module,
		action = EXCLUDED.action,
		name = EXCLUDED.name,
		slug = EXCLUDED.slug,
		description = EXCLUDED.description,
		updated_at = now()
	RETURNING id
),
owner_roles AS (
	SELECT id
	FROM roles
	WHERE role_name IN ('organization_owner')
		OR slug IN ('organization_owner')
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT
	owner_roles.id,
	upserted_permissions.id,
	'organization',
	now()
FROM owner_roles
CROSS JOIN upserted_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;

-- member gets read/create/update/send (day-to-day quotation/invoice work)
-- but not delete/approve/reject/mark_paid/cancel — financial-outcome and
-- destructive actions stay owner-only, matching the pattern set in
-- migrations/000081 (Deal) and 000078 (Company/Contact/Lead).
WITH member_permission_names(permission_name) AS (
	VALUES
		('quotation.read'), ('quotation.create'), ('quotation.update'), ('quotation.send'),
		('invoice.read'), ('invoice.create'), ('invoice.update'), ('invoice.send')
),
member_roles AS (
	SELECT id
	FROM roles
	WHERE role_name = 'member' OR slug = 'member'
),
target_permissions AS (
	SELECT id, permission_name
	FROM permissions
	WHERE permission_name IN (SELECT permission_name FROM member_permission_names)
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT
	member_roles.id,
	target_permissions.id,
	'organization',
	now()
FROM member_roles
CROSS JOIN target_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;
