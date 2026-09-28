-- Permissions for CRM email (user_mailboxes). A mailbox always belongs to
-- the user who connected it; nobody reads another user's mailbox, so there is
-- no read_all. organization_owner, super_admin, and member get both.
WITH email_permissions(permission_name, module, action, description) AS (
	VALUES
		('email.read', 'email', 'read', 'Read emails of own connected mailboxes'),
		('email.send', 'email', 'send', 'Connect own mailboxes and send email')
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
	FROM email_permissions
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
full_access_roles AS (
	SELECT id
	FROM roles
	WHERE role_name IN ('organization_owner', 'super_admin')
		OR slug IN ('organization_owner', 'super_admin')
)
INSERT INTO role_permissions (role_id, permission_id, scope, granted_at)
SELECT
	full_access_roles.id,
	upserted_permissions.id,
	'organization',
	now()
FROM full_access_roles
CROSS JOIN upserted_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;

WITH member_permission_names(permission_name) AS (
	VALUES
		('email.read'),
		('email.send')
),
member_roles AS (
	SELECT id
	FROM roles
	WHERE role_name = 'member' OR slug = 'member'
),
target_permissions AS (
	SELECT id
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
