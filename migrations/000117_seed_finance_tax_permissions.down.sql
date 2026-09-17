DELETE FROM role_permissions
WHERE permission_id IN (
	SELECT id FROM permissions WHERE permission_name IN (
		'platform.finance.tax.read',
		'platform.finance.tax.manage'
	)
);

DELETE FROM permissions WHERE permission_name IN (
	'platform.finance.tax.read',
	'platform.finance.tax.manage'
);
