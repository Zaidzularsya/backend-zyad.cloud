-- Seeds permissions for the Fase 2 CRM resources (Pipeline, Deal) — matching
-- what internal/modules/crm actually enforces today. See
-- migrations/000078_seed_crm_company_contact_lead_permissions.up.sql for the
-- same deviations this migration repeats: no super_admin grant (CRM is
-- customer-tenant-only), module='crm' with bare permission_name per
-- SKILLS.md's matrix.
WITH crm_permissions(permission_name, module, action, description) AS (
	VALUES
		('pipeline.read', 'crm', 'read', 'Read CRM pipelines'),
		('pipeline.create', 'crm', 'create', 'Create CRM pipelines'),
		('pipeline.update', 'crm', 'update', 'Update CRM pipelines'),
		('pipeline.delete', 'crm', 'delete', 'Delete CRM pipelines'),
		('pipeline.restore', 'crm', 'restore', 'Restore soft-deleted CRM pipelines'),
		('pipeline.archive', 'crm', 'archive', 'Archive CRM pipelines'),
		('pipeline.configure_stage', 'crm', 'configure_stage', 'Configure CRM pipeline stages'),
		('deal.read', 'crm', 'read', 'Read CRM deals'),
		('deal.create', 'crm', 'create', 'Create CRM deals'),
		('deal.update', 'crm', 'update', 'Update CRM deals'),
		('deal.delete', 'crm', 'delete', 'Delete CRM deals'),
		('deal.move_stage', 'crm', 'move_stage', 'Move CRM deals between pipeline stages'),
		('deal.close_won', 'crm', 'close_won', 'Close CRM deals as won'),
		('deal.close_lost', 'crm', 'close_lost', 'Close CRM deals as lost'),
		('deal.approve_discount', 'crm', 'approve_discount', 'Approve discount on CRM deals')
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

-- member gets non-destructive operational access: read/create/update on both
-- resources, plus move_stage (working a deal through the pipeline day-to-day).
-- Withheld from member: delete/restore/archive/configure_stage (pipeline
-- structure changes) and close_won/close_lost/approve_discount (financial
-- outcomes) — owner-only, matching docs/reference-crm.md "Permission Seed".
WITH member_permission_names(permission_name) AS (
	VALUES
		('pipeline.read'),
		('deal.read'), ('deal.create'), ('deal.update'), ('deal.move_stage')
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
