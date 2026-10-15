-- Permissions for the whatsapp module (docs/reference-whatsapp.md "Security
-- Baseline"). organization_owner and super_admin (platform organization) get
-- everything; member gets read + send. Conversation reads without
-- whatsapp.conversation.read_all are limited to the member's own assignments
-- in the service layer.
WITH whatsapp_permissions(permission_name, module, action, description) AS (
	VALUES
		('whatsapp.session.read', 'whatsapp', 'read', 'Read WhatsApp sessions'),
		('whatsapp.session.manage', 'whatsapp', 'manage', 'Create, pair, start, stop, and delete WhatsApp sessions'),
		('whatsapp.conversation.read', 'whatsapp', 'read', 'Read own WhatsApp conversations'),
		('whatsapp.conversation.read_all', 'whatsapp', 'read_all', 'Read all WhatsApp conversations in the organization'),
		('whatsapp.conversation.assign', 'whatsapp', 'assign', 'Assign WhatsApp conversations to a user'),
		('whatsapp.message.send', 'whatsapp', 'send', 'Send WhatsApp messages')
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
	FROM whatsapp_permissions
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
		('whatsapp.session.read'),
		('whatsapp.conversation.read'),
		('whatsapp.message.send')
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
