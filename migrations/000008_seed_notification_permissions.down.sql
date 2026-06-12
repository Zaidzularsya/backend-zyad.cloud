WITH notification_permissions(permission_name) AS (
	VALUES
		('notification_template.read'),
		('notification_template.create'),
		('notification_template.update'),
		('notification_template.delete'),
		('notification_template.preview'),
		('notification_template.activate'),
		('notification_template.deactivate'),
		('notification_template.archive'),
		('notification_template.clone'),
		('notification_variable.read'),
		('notification_log.read'),
		('notification_log.retry'),
		('notification_log.cancel'),
		('notification_preference.read'),
		('notification_preference.update'),
		('notification_preference.manage')
),
deleted_role_permissions AS (
	DELETE FROM role_permissions
	WHERE permission_id IN (
		SELECT p.id
		FROM permissions p
		JOIN notification_permissions np ON np.permission_name = p.permission_name
	)
	RETURNING permission_id
)
DELETE FROM permissions
WHERE permission_name IN (
	SELECT permission_name
	FROM notification_permissions
);
