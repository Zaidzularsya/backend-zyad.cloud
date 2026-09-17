DELETE FROM role_permissions
WHERE permission_id IN (
	SELECT id FROM permissions WHERE module = 'finance'
);

DELETE FROM permissions WHERE module = 'finance';
