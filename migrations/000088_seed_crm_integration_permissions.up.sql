-- Seeds permissions for the Fase 5 CRM resource (Integration) — the last of
-- the 10 SKILLS.md resources. See migrations/000078_...permissions.up.sql
-- for the no-super_admin-grant deviation this migration repeats.
WITH crm_permissions(permission_name, module, action, description) AS (
	VALUES
		('integration.read', 'crm', 'read', 'Read CRM integrations'),
		('integration.create', 'crm', 'create', 'Create CRM integrations'),
		('integration.update', 'crm', 'update', 'Update CRM integrations'),
		('integration.delete', 'crm', 'delete', 'Delete CRM integrations'),
		('integration.connect', 'crm', 'connect', 'Connect/activate CRM integrations'),
		('integration.view_secret', 'crm', 'view_secret', 'View decrypted CRM integration secrets'),
		('integration.update_secret', 'crm', 'update_secret', 'Update CRM integration secrets')
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

-- member gets read-only visibility (integrations touch external secrets and
-- webhooks — connecting/editing/viewing secrets stays owner-only, unlike the
-- more permissive member grants on the other 9 CRM resources).
WITH member_permission_names(permission_name) AS (
	VALUES ('integration.read')
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
