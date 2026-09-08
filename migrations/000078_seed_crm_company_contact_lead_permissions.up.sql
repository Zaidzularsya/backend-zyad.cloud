-- Seeds permissions for the Fase 1 CRM resources (Company, Contact, Lead)
-- only — matching what internal/modules/crm actually enforces today. The
-- remaining actions from the SKILLS.md matrix (import/export/merge/archive,
-- and a separate "customer" permission surface) are deferred; see
-- docs/reference-crm.md "Fase Implementasi" and "Non-Goals".
--
-- Deviation from the landing/organization permission seed pattern (see
-- migrations/000034_seed_landing_permissions.up.sql): CRM intentionally does
-- NOT grant super_admin (the platform role) any of these permissions — CRM
-- is a customer-tenant-only module (docs/reference-crm.md "Tenant Boundary").
WITH crm_permissions(permission_name, module, action, description) AS (
	VALUES
		('company.read', 'crm', 'read', 'Read CRM companies'),
		('company.create', 'crm', 'create', 'Create CRM companies'),
		('company.update', 'crm', 'update', 'Update CRM companies'),
		('company.delete', 'crm', 'delete', 'Delete CRM companies'),
		('company.restore', 'crm', 'restore', 'Restore soft-deleted CRM companies'),
		('contact.read', 'crm', 'read', 'Read CRM contacts'),
		('contact.create', 'crm', 'create', 'Create CRM contacts'),
		('contact.update', 'crm', 'update', 'Update CRM contacts'),
		('contact.delete', 'crm', 'delete', 'Delete CRM contacts'),
		('contact.restore', 'crm', 'restore', 'Restore soft-deleted CRM contacts'),
		('lead.read', 'crm', 'read', 'Read CRM leads'),
		('lead.create', 'crm', 'create', 'Create CRM leads'),
		('lead.update', 'crm', 'update', 'Update CRM leads'),
		('lead.delete', 'crm', 'delete', 'Delete CRM leads'),
		('lead.restore', 'crm', 'restore', 'Restore soft-deleted CRM leads'),
		('lead.assign', 'crm', 'assign', 'Assign CRM leads to a user'),
		('lead.convert', 'crm', 'convert', 'Convert a CRM lead into a contact/company')
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

-- member gets a non-destructive operational subset: read/create/update on
-- company & contact, plus the full lead working lifecycle (leads are
-- expected to be worked by any team member, not just the owner). Permission
-- rows already exist from the upsert above, so this only needs to grant.
WITH member_permission_names(permission_name) AS (
	VALUES
		('company.read'), ('company.create'), ('company.update'),
		('contact.read'), ('contact.create'), ('contact.update'),
		('lead.read'), ('lead.create'), ('lead.update'), ('lead.assign'), ('lead.convert')
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
