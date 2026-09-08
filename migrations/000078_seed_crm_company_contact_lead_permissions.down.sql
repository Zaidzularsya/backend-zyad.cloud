WITH seeded_permissions AS (
	SELECT id
	FROM permissions
	WHERE permission_name IN (
		'company.read', 'company.create', 'company.update', 'company.delete', 'company.restore',
		'contact.read', 'contact.create', 'contact.update', 'contact.delete', 'contact.restore',
		'lead.read', 'lead.create', 'lead.update', 'lead.delete', 'lead.restore', 'lead.assign', 'lead.convert'
	)
)
DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM seeded_permissions);

DELETE FROM permissions
WHERE permission_name IN (
	'company.read', 'company.create', 'company.update', 'company.delete', 'company.restore',
	'contact.read', 'contact.create', 'contact.update', 'contact.delete', 'contact.restore',
	'lead.read', 'lead.create', 'lead.update', 'lead.delete', 'lead.restore', 'lead.assign', 'lead.convert'
);
