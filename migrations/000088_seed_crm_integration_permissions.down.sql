WITH seeded_permissions AS (
	SELECT id
	FROM permissions
	WHERE permission_name IN (
		'integration.read', 'integration.create', 'integration.update', 'integration.delete',
		'integration.connect', 'integration.view_secret', 'integration.update_secret'
	)
)
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM seeded_permissions);

DELETE FROM permissions
WHERE permission_name IN (
	'integration.read', 'integration.create', 'integration.update', 'integration.delete',
	'integration.connect', 'integration.view_secret', 'integration.update_secret'
);
