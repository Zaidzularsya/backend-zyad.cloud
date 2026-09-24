DELETE FROM role_permissions
USING permissions
WHERE role_permissions.permission_id = permissions.id
	AND permissions.permission_name IN (
		'whatsapp.session.read', 'whatsapp.session.manage',
		'whatsapp.conversation.read', 'whatsapp.conversation.read_all', 'whatsapp.conversation.assign',
		'whatsapp.message.send'
	);

DELETE FROM permissions
WHERE permission_name IN (
	'whatsapp.session.read', 'whatsapp.session.manage',
	'whatsapp.conversation.read', 'whatsapp.conversation.read_all', 'whatsapp.conversation.assign',
	'whatsapp.message.send'
);
