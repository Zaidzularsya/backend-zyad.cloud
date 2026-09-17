DELETE FROM role_permissions
WHERE permission_id IN (
	SELECT id FROM permissions WHERE permission_name IN (
		'platform.finance.arap.read',
		'platform.finance.arap.manage'
	)
);

DELETE FROM permissions WHERE permission_name IN (
	'platform.finance.arap.read',
	'platform.finance.arap.manage'
);
