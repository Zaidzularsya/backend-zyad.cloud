WITH landing_permissions AS (
	SELECT id FROM permissions WHERE module = 'landing'
)
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM landing_permissions);

DELETE FROM permissions WHERE module = 'landing';
