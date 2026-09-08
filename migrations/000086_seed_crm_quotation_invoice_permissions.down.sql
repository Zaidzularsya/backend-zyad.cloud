WITH seeded_permissions AS (
	SELECT id
	FROM permissions
	WHERE permission_name IN (
		'quotation.read', 'quotation.create', 'quotation.update', 'quotation.delete',
		'quotation.send', 'quotation.approve', 'quotation.reject',
		'invoice.read', 'invoice.create', 'invoice.update', 'invoice.delete',
		'invoice.send', 'invoice.mark_paid', 'invoice.cancel'
	)
)
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM seeded_permissions);

DELETE FROM permissions
WHERE permission_name IN (
	'quotation.read', 'quotation.create', 'quotation.update', 'quotation.delete',
	'quotation.send', 'quotation.approve', 'quotation.reject',
	'invoice.read', 'invoice.create', 'invoice.update', 'invoice.delete',
	'invoice.send', 'invoice.mark_paid', 'invoice.cancel'
);
