WITH billing_permissions AS (
	SELECT id FROM permissions WHERE module = 'billing'
)
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM billing_permissions);

DELETE FROM permissions WHERE module = 'billing';
