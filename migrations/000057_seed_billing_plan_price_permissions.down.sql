DELETE FROM role_permissions
WHERE permission_id IN (
	SELECT id
	FROM permissions
	WHERE permission_name IN (
		'platform.billing.plan_price.read',
		'platform.billing.plan_price.manage'
	)
);

DELETE FROM permissions
WHERE permission_name IN (
	'platform.billing.plan_price.read',
	'platform.billing.plan_price.manage'
);
