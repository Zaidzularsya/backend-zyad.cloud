WITH seeded_permissions AS (
	SELECT id
	FROM permissions
	WHERE permission_name IN ('storage.object.read', 'storage.object.manage')
)
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM seeded_permissions);

DELETE FROM permissions
WHERE permission_name IN ('storage.object.read', 'storage.object.manage');
