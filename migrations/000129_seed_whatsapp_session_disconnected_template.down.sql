DELETE FROM notification_templates
WHERE code = 'whatsapp.session_disconnected'
	AND channel = 'email'
	AND locale = 'id-ID'
	AND version = 1
	AND is_system = true;
