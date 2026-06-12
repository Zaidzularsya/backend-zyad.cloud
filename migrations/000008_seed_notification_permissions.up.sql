WITH notification_permissions(permission_name, description) AS (
	VALUES
		('notification_template.read', 'Read notification templates'),
		('notification_template.create', 'Create notification templates'),
		('notification_template.update', 'Update notification templates'),
		('notification_template.delete', 'Delete notification templates'),
		('notification_template.preview', 'Preview notification templates'),
		('notification_template.activate', 'Activate notification templates'),
		('notification_template.deactivate', 'Deactivate notification templates'),
		('notification_template.archive', 'Archive notification templates'),
		('notification_template.clone', 'Clone notification templates'),
		('notification_variable.read', 'Read notification template variables'),
		('notification_log.read', 'Read notification logs'),
		('notification_log.retry', 'Retry notification logs'),
		('notification_log.cancel', 'Cancel notification logs'),
		('notification_preference.read', 'Read notification preferences'),
		('notification_preference.update', 'Update notification preferences'),
		('notification_preference.manage', 'Manage notification preferences')
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
		split_part(permission_name, '.', 1),
		split_part(permission_name, '.', 2),
		permission_name,
		permission_name,
		description,
		now(),
		now()
	FROM notification_permissions
	ON CONFLICT (permission_name)
	DO UPDATE SET
		module = EXCLUDED.module,
		action = EXCLUDED.action,
		name = EXCLUDED.name,
		slug = EXCLUDED.slug,
		description = EXCLUDED.description,
		updated_at = now()
	RETURNING id
),
super_admin_role AS (
	INSERT INTO roles (
		role_name,
		slug,
		description,
		is_system,
		created_at,
		updated_at
	)
	VALUES (
		'super_admin',
		'super_admin',
		'Full platform administrator',
		true,
		now(),
		now()
	)
	ON CONFLICT (role_name)
	DO UPDATE SET
		slug = EXCLUDED.slug,
		description = COALESCE(NULLIF(roles.description, ''), EXCLUDED.description),
		is_system = true,
		updated_at = now()
	RETURNING id
)
INSERT INTO role_permissions (
	role_id,
	permission_id,
	scope,
	granted_at
)
SELECT
	super_admin_role.id,
	upserted_permissions.id,
	'all',
	now()
FROM super_admin_role
CROSS JOIN upserted_permissions
ON CONFLICT (role_id, permission_id)
DO UPDATE SET
	scope = EXCLUDED.scope,
	granted_at = EXCLUDED.granted_at;
