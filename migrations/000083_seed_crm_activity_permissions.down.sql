WITH seeded_permissions AS (
	SELECT id
	FROM permissions
	WHERE permission_name IN (
		'activity.read', 'activity.create', 'activity.update', 'activity.delete',
		'activity.complete', 'activity.cancel', 'activity.assign'
	)
)
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM seeded_permissions);

DELETE FROM permissions
WHERE permission_name IN (
	'activity.read', 'activity.create', 'activity.update', 'activity.delete',
	'activity.complete', 'activity.cancel', 'activity.assign'
);
