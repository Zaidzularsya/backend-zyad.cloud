-- Seeds permissions for the Fase 3 CRM resource (Activity) — matching
-- SKILLS.md's matrix exactly: read, create, update, delete, complete,
-- cancel, assign (no restore action for Activity, unlike Company/Contact/
-- Lead/Pipeline). See migrations/000078_seed_crm_company_contact_lead_permissions.up.sql
-- for the deviations this migration repeats (no super_admin grant).
WITH crm_permissions(permission_name, module, action, description) AS (
	VALUES
		('activity.read', 'crm', 'read', 'Read CRM activities'),
		('activity.create', 'crm', 'create', 'Create CRM activities'),
		('activity.update', 'crm', 'update', 'Update CRM activities'),
		('activity.delete', 'crm', 'delete', 'Delete CRM activities'),
		('activity.complete', 'crm', 'complete', 'Mark CRM activities as completed'),
		('activity.cancel', 'crm', 'cancel', 'Cancel CRM activities'),
		('activity.assign', 'crm', 'assign', 'Assign CRM activities to a user')
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

-- member gets the full activity working lifecycle except delete (a shared
-- interaction log entry shouldn't be silently removable by any team member).
WITH member_permission_names(permission_name) AS (
	VALUES
		('activity.read'), ('activity.create'), ('activity.update'),
		('activity.complete'), ('activity.cancel'), ('activity.assign')
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
